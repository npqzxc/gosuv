package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/go-yaml/yaml"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
	_ "github.com/shurcooL/vfsgen"
	"github.com/soopsio/gosuv/gops"
	"github.com/soopsio/kexec"

	"github.com/hpcloud/tail"
	"github.com/thinkeridea/go-extend/exstrings"
	"net"
	"strings"
	// "runtime"
)

var defaultGosuvDir string

func init() {
	defaultGosuvDir = os.Getenv("GOSUV_HOME_DIR")
	if defaultGosuvDir == "" {
		defaultGosuvDir = filepath.Join(UserHomeDir(), ".gosuv")
	}
	http.Handle("/res/", http.StripPrefix("/res/", http.FileServer(Assets))) // http.StripPrefix("/res/", Assets))
}

type Supervisor struct {
	ConfigDir string

	names   []string // order of programs
	pgMap   map[string]Program
	procMap map[string]*Process
	mu      sync.Mutex
	eventB  *WriteBroadcaster
}

func (s *Supervisor) programs() []Program {
	pgs := make([]Program, 0, len(s.names))
	for _, name := range s.names {
		pgs = append(pgs, s.pgMap[name])
	}
	return pgs
}

func (s *Supervisor) procs(key string) []*Process {
	ps := make([]*Process, 0, len(s.names))
	for _, name := range s.names {
		if len(key) > 0 && key != name {
			continue
		}
		ps = append(ps, s.procMap[name])
		if len(key) > 0 {
			break
		}
	}
	return ps
}

func (s *Supervisor) operateProcs(ope FSMEvent) {
	for _, name := range s.names {
		s.operationPro(name, ope)
	}
}

func (s *Supervisor) programPath() string {
	return filepath.Join(s.ConfigDir, "programs.yml")
}

func (s *Supervisor) newProcess(pg Program) *Process {
	p := NewProcess(pg)
	origFunc := p.StateChange
	p.StateChange = func(oldState, newState FSMState) {
		s.broadcastEvent(fmt.Sprintf("%s state: %s -> %s", p.Name, string(oldState), string(newState)))
		origFunc(oldState, newState)
	}
	return p
}

func (s *Supervisor) broadcastEvent(event string) {
	s.eventB.Write([]byte(event))
}

// update 新增返回chan，用于websocket内部关闭，避免浪费协程和chan
func (s *Supervisor) addStatusChangeListener(c chan string) (name string) {
	name = fmt.Sprintf("%d", time.Now().UnixNano())
	sChan := s.eventB.NewChanString(name)
	GoSub(func() {
		// LOG("----start addStatusChangeListener=[%p]",s)
		// defer func(){
		// LOG("---close addStatusChangeListener=[%p]",s)
		// }()
		for msg := range sChan {
			c <- msg
		}
	})
	return
}

// Send Stop signal and wait program stops
func (s *Supervisor) stopAndWait(name string) error {
	p, ok := s.procMap[name]
	if !ok {
		return errors.New("No such program")
	}
	if !p.IsRunning() {
		return nil
	}
	c := make(chan string, 1)
	// s.addStatusChangeListener(c)
	keyname := s.addStatusChangeListener(c)
	defer func() {
		s.eventB.CloseWriter(keyname)
		close(c)
	}()
	// p.stopCommand()
	// 停止任务
	p.Operate(StopEvent)
	for {
		select {
		case <-c:
			if !p.IsRunning() {
				return nil
			}
		case <-time.After(1 * time.Second): // In case some event not catched
			if !p.IsRunning() {
				return nil
			}
		}
	}
}

func (s *Supervisor) addOrUpdateProgram(pg Program) error {
	// defer s.broadcastEvent(pg.Name + " add or update")
	if err := pg.Check(); err != nil {
		return err
	}
	origPg, ok := s.pgMap[pg.Name]
	if ok {
		if reflect.DeepEqual(origPg, pg) {
			return nil
		}
		s.broadcastEvent(pg.Name + " update")
		log.Println("Update:", pg.Name)
		origProc := s.procMap[pg.Name]
		isRunning := origProc.IsRunning()
		GoSub(func() {
			// LOG("----start addOrUpdateProgram=[%p]",s)
			// defer func(){
			// LOG("---close addOrUpdateProgram=[%p]",s)
			// }()
			s.stopAndWait(origProc.Name)

			newProc := s.newProcess(pg)
			s.procMap[pg.Name] = newProc
			s.pgMap[pg.Name] = pg // update origin
			if isRunning {
				newProc.Operate(StartEvent)
			}
			s.saveDB()
		})
	} else {
		s.names = append(s.names, pg.Name)
		s.pgMap[pg.Name] = pg
		s.procMap[pg.Name] = s.newProcess(pg)
		s.broadcastEvent(pg.Name + " added")
	}
	return nil
}

// Check
// - Yaml format
// - Duplicated program
func (s *Supervisor) readConfigFromDB() (pgs []Program, err error) {
	data, err := ioutil.ReadFile(s.programPath())
	if err != nil {
		data = []byte("")
	}
	pgs = make([]Program, 0)
	if err = yaml.Unmarshal(data, &pgs); err != nil {
		return nil, err
	}
	visited := map[string]bool{}
	for _, pg := range pgs {
		if visited[pg.Name] {
			return nil, fmt.Errorf("Duplicated program name: %s", pg.Name)
		}
		visited[pg.Name] = true
	}
	return
}

func (s *Supervisor) loadDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pgs, err := s.readConfigFromDB()
	if err != nil {
		return err
	}
	// add or update program
	visited := map[string]bool{}
	names := make([]string, 0, len(pgs))
	for _, pg := range pgs {
		names = append(names, pg.Name)
		visited[pg.Name] = true
		s.addOrUpdateProgram(pg)
	}
	s.names = names
	// delete not exists program
	for _, pg := range s.pgMap {
		if visited[pg.Name] {
			continue
		}
		s.removeProgram(pg.Name)
		// name := pg.Name
		// log.Printf("stop before delete program: %s", name)
		// s.stopAndWait(name)
		// delete(s.procMap, name)
		// delete(s.pgMap, name)
		// s.broadcastEvent(name + " deleted")
	}
	return nil
}

func (s *Supervisor) saveDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := yaml.Marshal(s.programs())
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.programPath(), data, 0644)
}

func (s *Supervisor) removeProgram(name string) {
	names := make([]string, 0, len(s.names))
	for _, pName := range s.names {
		if pName == name {
			continue
		}
		names = append(names, pName)
	}
	s.names = names
	log.Printf("stop before delete program: %s", name)
	s.stopAndWait(name)
	delete(s.procMap, name)
	delete(s.pgMap, name)
	s.broadcastEvent(name + " deleted")
}

type WebConfig struct {
	User    string
	Version string
}

func (s *Supervisor) renderHTML(w http.ResponseWriter, name string, data interface{}) {
	file, err := Assets.Open(name + ".html")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	body, _ := ioutil.ReadAll(file)

	if data == nil {
		wc := WebConfig{}
		wc.Version = version
		user, err := user.Current()
		if err == nil {
			wc.User = user.Username
		}
		data = wc
	}
	w.Header().Set("Content-Type", "text/html")
	template.Must(template.New("t").Delims("[[", "]]").Parse(string(body))).Execute(w, data)
}

type JSONResponse struct {
	Status int         `json:"status"`
	Value  interface{} `json:"value"`
}

func (s *Supervisor) renderJSON(w http.ResponseWriter, data JSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	bytes, _ := json.Marshal(data)
	w.Write(bytes)
}

func (s *Supervisor) hIndex(w http.ResponseWriter, r *http.Request) {
	s.renderHTML(w, "index", nil)
}

func (s *Supervisor) hSetting(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	s.renderHTML(w, "setting", map[string]string{
		"Name": name,
	})
}

func (s *Supervisor) hStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(map[string]interface{}{
		"status": 0,
		"value":  "server is running",
	})
	w.Write(data)
}

func (s *Supervisor) hShutdown(w http.ResponseWriter, r *http.Request) {
	s.Close()
	s.renderJSON(w, JSONResponse{
		Status: 0,
		Value:  "gosuv server has been shutdown",
	})
	GoSub(func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	})
}

func (s *Supervisor) hReload(w http.ResponseWriter, r *http.Request) {
	err := s.loadDB()
	log.Println("reload config file")
	if err == nil {
		s.renderJSON(w, JSONResponse{
			Status: 0,
			Value:  "load config success",
		})
	} else {
		s.renderJSON(w, JSONResponse{
			Status: 1,
			Value:  err.Error(),
		})
	}
}

func (s *Supervisor) hGetProgramList(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(s.procs(""))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Supervisor) hGetProgramStatus(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	data, err := json.Marshal(s.procs(name))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Supervisor) hGetProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	proc, ok := s.procMap[name]
	if !ok {
		s.renderJSON(w, JSONResponse{
			Status: 1,
			Value:  "program not exists",
		})
		return
	} else {
		s.renderJSON(w, JSONResponse{
			Status: 0,
			Value:  proc,
		})
	}
}

// HSelectStartProgram H SelectStartProgram
func (s *Supervisor) HSelectStartProgram(w http.ResponseWriter, r *http.Request) {
	//glog.Error("----------xxxxxxxxxxx-------------")
	//glog.Error("error glog")
	//glog.Errorf("x 的数据类型是: %T\n",body)
	//glog.Error("zifu 的数据类型是:",reflect.TypeOf(body))
	//glog.Errorf("zifu 的数据类型是: %T",body)

	body, _ := ioutil.ReadAll(r.Body)
	var jsonMap []string
	json.Unmarshal(body, &jsonMap)
	for _, v := range jsonMap {
		s.operationPro(v, StartEvent)
	}
	var data []byte
	data, _ = json.Marshal(map[string]interface{}{
		"status": 0,
		"name":   "SUCCESS",
	})
	w.Write(data)
}

// HSelectStopProgram H SelectStopProgram
func (s *Supervisor) HSelectStopProgram(w http.ResponseWriter, r *http.Request) {
	//glog.Error("----------xxxxxxxxxxx-------------")
	//glog.Error("error glog")
	//glog.Errorf("x 的数据类型是: %T\n",body)
	//glog.Error("zifu 的数据类型是:",reflect.TypeOf(body))
	//glog.Errorf("zifu 的数据类型是: %T",body)

	body, _ := ioutil.ReadAll(r.Body)
	var jsonMap []string
	json.Unmarshal(body, &jsonMap)
	for _, v := range jsonMap {
		s.operationPro(v, StopEvent)
	}
	var data []byte
	data, _ = json.Marshal(map[string]interface{}{
		"status": 0,
		"name":   "SUCCESS",
	})
	w.Write(data)

}

func (s *Supervisor) hAddProgram(w http.ResponseWriter, r *http.Request) {
	retries, err := strconv.Atoi(r.FormValue("retries"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	pg := Program{
		Name:         r.FormValue("name"),
		Command:      r.FormValue("command"),
		Dir:          r.FormValue("dir"),
		User:         r.FormValue("user"),
		StartAuto:    r.FormValue("autostart") == "on",
		StartRetries: retries,
		CustomLog:    r.FormValue("custom_log"),
		ConfigPath:   r.FormValue("config_path"),
		// TODO: missing other values
	}
	if pg.Dir == "" {
		pg.Dir = "/"
	}
	if err := pg.Check(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var data []byte
	if _, ok := s.pgMap[pg.Name]; ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Program %s already exists", strconv.Quote(pg.Name)),
		})
	} else {
		if err := s.addOrUpdateProgram(pg); err != nil {
			data, _ = json.Marshal(map[string]interface{}{
				"status": 1,
				"error":  err.Error(),
			})
		} else {
			s.saveDB()
			data, _ = json.Marshal(map[string]interface{}{
				"status": 0,
			})
		}
	}
	w.Write(data)
}

func (s *Supervisor) hUpdateProgram(w http.ResponseWriter, r *http.Request) {
	// name := mux.Vars(r)["name"]
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	pg := Program{}
	err := json.NewDecoder(r.Body).Decode(&pg)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 1,
			"error":  err.Error(),
		})
		return
	}
	err = s.addOrUpdateProgram(pg)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 2,
			"error":  err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      0,
		"description": "program updated",
	})
}

func (s *Supervisor) hDelProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	w.Header().Set("Content-Type", "application/json")
	var data []byte
	if _, ok := s.pgMap[name]; !ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Program %s not exists", strconv.Quote(name)),
		})
	} else {
		s.removeProgram(name)
		s.saveDB()
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
		})
	}
	w.Write(data)
}

func (s *Supervisor) hStartProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "all" {
		s.operateProcs(StartEvent)
		var data []byte
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
			"name":   "SUCCESS",
		})
		w.Write(data)
		return
	}
	proc, ok := s.procMap[name]
	var data []byte
	if !ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		})
	} else {
		proc.Operate(StartEvent)
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
			"name":   name,
		})
	}
	w.Write(data)
}

// operationPro
func (s *Supervisor) operationPro(name string, event FSMEvent) {
	proc, ok := s.procMap[name]
	if ok {
		proc.Operate(event)
	}
}

func (s *Supervisor) hStopProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "all" {
		s.operateProcs(StopEvent)
		var data []byte
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
			"name":   "SUCCESS",
		})
		w.Write(data)
		return
	}
	proc, ok := s.procMap[name]
	var data []byte
	if !ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		})
	} else {
		proc.Operate(StopEvent)
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
			"name":   name,
		})
	}
	w.Write(data)
}

// hRestartProgram
func (s *Supervisor) hRestartProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	proc, ok := s.procMap[name]
	var data []byte
	if !ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		})
	} else {
		proc.Operate(RestartEvent)
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
			"name":   name,
		})
	}
	w.Write(data)
}

// hGetConfig
func (s *Supervisor) hGetConfig(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	proc, ok := s.procMap[name]
	//fmt.Println(proc)
	var data []byte
	if !ok {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		})
	} else {
		if proc.ConfigPath != "" {
			longConfigPath := filepath.Join(proc.Dir, proc.ConfigPath)
			b, e := ioutil.ReadFile(longConfigPath)
			if e != nil {
				fmt.Println("read file error")
				return
			}
			data = b
		}
	}
	w.Write(data)
}

// hUpdateConfig
func (s *Supervisor) hUpdateConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	name := mux.Vars(r)["name"]
	proc, _ := s.procMap[name]
	var data []byte
	body, _ := ioutil.ReadAll(r.Body)
	var jsonMap string
	json.Unmarshal(body, &jsonMap)
	longConfigPath := filepath.Join(proc.Dir, proc.ConfigPath)
	if err := ioutil.WriteFile(longConfigPath, []byte(jsonMap), 0666); err != nil {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("config %s is error", strconv.Quote(name)),
		})
	} else {
		data, _ = json.Marshal(map[string]interface{}{
			"status": 0,
		})
	}
	w.Write(data)
}

// hWebhook
func (s *Supervisor) hWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, category := vars["name"], vars["category"]
	proc, ok := s.procMap[name]
	if !ok {
		http.Error(w, fmt.Sprintf("proc %s not exist", strconv.Quote(name)), http.StatusForbidden)
		return
	}
	hook := proc.Program.WebHook
	if category == "github" {
		gh := hook.Github
		_ = gh.Secret
		isRunning := proc.IsRunning()
		s.stopAndWait(name)
		GoSub(func() {
			// LOG("----start hWebhook=[%p]",s)
			// defer func(){
			// LOG("---close hWebhook=[%p]",s)
			// }()
			cmd := kexec.CommandString(hook.Command)
			cmd.Dir = proc.Program.Dir
			cmd.Stdout = proc.Output
			cmd.Stderr = proc.Output
			err := GoTimeout(cmd.Run, time.Duration(hook.Timeout)*time.Second)
			if err == ErrGoTimeout {
				cmd.Terminate(syscall.SIGTERM)
			}
			if err != nil {
				log.Warnf("webhook command error: %v", err)
				// Trigger pushover notification
			}
			if isRunning {
				proc.Operate(StartEvent)
			}
		})
		io.WriteString(w, "success triggered")
	} else {
		log.Warnf("unknown webhook category: %v", category)
	}
}

var upgrader = websocket.Upgrader{}

func (s *Supervisor) wsEvents(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	ch := make(chan string, 1)
	keyname := s.addStatusChangeListener(ch)
	defer func() {
		s.eventB.CloseWriter(keyname)
		close(ch)
	}()
	GoSub(func() {
		// LOG("----start ws/events=[%p]",s)
		// defer func(){
		// LOG("---close ws/events=[%p]",s)
		// }()
		_, _ = <-ch // ignore the history messages
		for message := range ch {
			// Question: type 1 ?
			c.WriteMessage(1, []byte(message))
		}
		// s.eventB.RemoveListener(ch)
	})
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", mt, err)
			break
		}
		log.Printf("recv: %v %s", mt, message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func wsRecv(c *websocket.Conn, outch chan interface{}) {
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			if neterr, ok := err.(net.Error); !ok || !neterr.Timeout() {
				close(outch)
				return
			}
		}
	}
}
func (s *Supervisor) wsLog(w http.ResponseWriter, r *http.Request) {
	// LOG("new wsLog")
	name := mux.Vars(r)["name"]
	proc, ok := s.procMap[name]
	if !ok {
		log.Println("No such process")
		// TODO: raise error here?
		return
	}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	outch := make(chan interface{}, 1)
	if proc.CustomLog == "" {
		ch := proc.Output.NewChanString(r.RemoteAddr)
		defer proc.Output.CloseWriter(r.RemoteAddr)
		GoSub(func() { wsRecv(c, outch) })

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					break
				}
				// LOG("proc.Output.CloseWriter(r.RemoteAddr),data=[%q]",data)
				err := c.WriteMessage(1, []byte(data))
				if err != nil {
					// LOG("proc.Output.CloseWriter(r.RemoteAddr)")
					return
				}
			case <-outch:
				return
			}
		}
	} else {
		longCustomLog := filepath.Join(proc.Dir, proc.CustomLog)
		outmsg := make([]string, 0, 51)
		t, err := tail.TailFile(longCustomLog, tail.Config{Follow: false})
		for line := range t.Lines {
			outmsg = append(outmsg, line.Text+"\n")
			if len(outmsg) > 50 {
				outmsg = outmsg[1:]
			}
		}
		if len(outmsg) > 0 {
			err := c.WriteMessage(1, exstrings.Bytes(strings.Join(outmsg, "")))
			if err != nil {
				return
			}
		}
		outmsg = nil
		tails, err := tail.TailFile(longCustomLog, tail.Config{
			ReOpen:    true,                                 //true则文件被删掉阻塞等待新建该文件，false则文件被删掉时程序结束
			Follow:    true,                                 //true则一直阻塞并监听指定文件，false则一次读完就结束程序
			Location:  &tail.SeekInfo{Offset: 0, Whence: 2}, // 从文件的哪个地方开始读
			MustExist: false,                                //true则没有找到文件就报错并结束，false则没有找到文件就阻塞保持住
			Poll:      true,                                 // 使用Linux的Poll函数，poll的作用是把当前的文件指针挂到等待队列
			Logger:    tail.DiscardingLogger,                // Logger, when nil, is set to tail.DefaultLogge, To disable logging: set field to tail.DiscardingLogger
		})

		if err != nil {
			fmt.Println("tail file err:", err)
			return
		}

		defer tails.Kill(nil)
		GoSub(func() { wsRecv(c, outch) })

		var msg *tail.Line
		var ok bool
		for {
			select {
			case msg, ok = <-tails.Lines:
				if !ok {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				err := c.WriteMessage(1, []byte(msg.Text+"\n"))
				if err != nil {
					return
				}
			case <-outch:
				return
			}
		}
	}
}

// Performance
func (s *Supervisor) wsPerf(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	name := mux.Vars(r)["name"]
	proc, ok := s.procMap[name]
	if !ok {
		log.Println("No such process")
		// TODO: raise error here?
		return
	}
	for {
		// c.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if proc.cmd == nil || proc.cmd.Process == nil {
			log.Println("process not running")
			return
		}
		pid := proc.cmd.Process.Pid
		ps, err := gops.NewProcess(pid)
		if err != nil {
			break
		}
		mainPinfo, err := ps.ProcInfo()
		if err != nil {
			break
		}
		pi := ps.ChildrenProcInfo(true)
		pi.Add(mainPinfo)

		err = c.WriteJSON(pi)
		if err != nil {
			break
		}
		time.Sleep(700 * time.Millisecond)
	}
}

func (s *Supervisor) Close() {
	for _, proc := range s.procMap {
		s.stopAndWait(proc.Name)
	}
	log.Println("server closed")
}

func (s *Supervisor) catchExitSignal() {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	GoSub(func() {
		for sig := range sigC {
			if sig == syscall.SIGHUP {
				log.Println("Receive SIGHUP, just ignore")
				continue
			}
			log.Printf("Got signal: %v, stopping all running process\n", sig)
			s.Close()
			break
		}
		os.Exit(0)
	})
}

func (s *Supervisor) AutoStartPrograms() {
	for _, proc := range s.procMap {
		if proc.Program.StartAuto {
			log.Printf("auto start %s", strconv.Quote(proc.Name))
			proc.Operate(StartEvent)
		}
	}
}

func newSupervisorHandler() (suv *Supervisor, hdlr http.Handler, err error) {
	suv = &Supervisor{
		ConfigDir: filepath.Join(defaultGosuvDir, "conf"),
		pgMap:     make(map[string]Program, 0),
		procMap:   make(map[string]*Process, 0),
		eventB:    NewWriteBroadcaster(4 * 1024),
	}
	if err = suv.loadDB(); err != nil {
		return
	}
	suv.catchExitSignal()

	r := mux.NewRouter()
	r.HandleFunc("/", suv.hIndex)
	r.HandleFunc("/settings/{name}", suv.hSetting)

	r.HandleFunc("/api/status", suv.hStatus)
	r.HandleFunc("/api/shutdown", suv.hShutdown).Methods("POST")
	r.HandleFunc("/api/reload", suv.hReload).Methods("POST")

	r.HandleFunc("/api/programs", suv.hGetProgramList).Methods("GET")
	r.HandleFunc("/api/programs/{name}/status", suv.hGetProgramStatus).Methods("GET")
	r.HandleFunc("/api/programs/{name}", suv.hGetProgram).Methods("GET")
	r.HandleFunc("/api/programs/{name}", suv.hDelProgram).Methods("DELETE")
	r.HandleFunc("/api/programs/{name}", suv.hUpdateProgram).Methods("PUT")
	r.HandleFunc("/api/programs", suv.hAddProgram).Methods("POST")
	r.HandleFunc("/api/programs/selectStartProgram", suv.HSelectStartProgram).Methods("POST")
	r.HandleFunc("/api/programs/selectStopProgram", suv.HSelectStopProgram).Methods("POST")
	r.HandleFunc("/api/programs/{name}/start", suv.hStartProgram).Methods("POST")
	r.HandleFunc("/api/programs/{name}/stop", suv.hStopProgram).Methods("POST")
	r.HandleFunc("/api/programs/{name}/restart", suv.hRestartProgram).Methods("POST")
	r.HandleFunc("/ws/events", suv.wsEvents)
	r.HandleFunc("/ws/logs/{name}", suv.wsLog)
	r.HandleFunc("/ws/perfs/{name}", suv.wsPerf)

	r.HandleFunc("/webhooks/{name}/{category}", suv.hWebhook).Methods("POST")
	r.HandleFunc("/api/config/{name}", suv.hGetConfig).Methods("GET")
	r.HandleFunc("/api/config/{name}", suv.hUpdateConfig).Methods("PUT")
	return suv, r, nil
}
