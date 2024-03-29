# gosuv
[![Build Status](https://travis-ci.org/codeskyblue/gosuv.svg)](https://travis-ci.org/codeskyblue/gosuv)

[中文README](README_ZH.md) 不是很全，能看懂英文的还是尽量看英文

## current is in beta
Process management writtern by golang, inspired by python-supervisor

## So why write another supervisor?
I have been using python-supervisor for many years and there are something uncomfortable feelings.

1. Log can't contains ANSI color chars
1. The configuration file can add on the web, often forgot some settings.
1. `supervisorctl reload` will cause supervisord restarted
1. Hard to set status change to fatal notifications.
1. No process performance monitor page.
1. Program starts with no common environ, eg, missing HOME and USER variable
1. Kill process default is not group kill which make sub process still running.
1. More... will added when I think of it.

## Features

* [x] Web control page
	
  * [x] Start, Stop, Tail, Reload
  * [x] Realtime log
	* [x] Add program support
	* [x] Edit support
	* [x] Delete support
	* [x] Memory and CPU monitor
	* [ ] Path auto complete <https://github.com/twitter/typeahead.js>

* [x] HTTP Basic auth
* [x] Github webhook
* [ ] Single log page, include search support
* [ ] 中文文档

## Requirements
Go version at least `1.6+`

## Installation
### Binaries
The fastest way is run with. Default install location is `/usr/local/bin`, change env-var `BINDIR` will also change install location.

```
curl https://raw.githubusercontent.com/codeskyblue/gosuv/master/get.sh | bash
```

Or just download binaries

<https://github.com/codeskyblue/gosuv/releases>

Thanks to [goreleaser](https://github.com/goreleaser/goreleaser) which makes publish binaries automaticly.

### Build from source
```sh
go get -d github.com/codeskyblue/gosuv
cd $GOPATH/src/github.com/codeskyblue/gosuv
go generate # package html resources into go
go build -tags vfs
#### mac打linux包
export GOOS=linux  && go build -tags vfs
```

## Quick start
After you installed gosuv, the first thing is to start server.

```sh
gosuv start-server
gosuv shutdown
```

Basic operations

```sh
$ gosuv status
PROGRAM NAME            STATUS
test                    running
test_again              stopped

$ gosuv stop test
$ gosuv start test
```

Open web <http://localhost:11313> to see the manager page. And follow the gif to add a program to gosuv.


![gosuv web](docs/gosuv.gif)

## Configuration
Default config file stored in directory `$HOME/.gosuv/`, Set env-var `GOSUV_HOME_DIR` can change config file store directory.

- file `programs.yml` contains all program settings.
- file `config.yml` contains server config

File `config.yml` can be generated by `gosuv conftest`

Example config.yaml

```
server:
  httpauth:
    enabled: true
    username: admin
    password: admin
  addr: :8083
  name: 10.10.99.177
  master: ""
notifications:
  dingtalk:
    groups:
    - secret: c1b8032******************aa736a
      mobile:
      - "153********"
client:
  server_url: http://admin:admin@localhost:8083
```

master 的支持来自  https://github.com/ihaiker/distributed-gosuv
Logs can be found in `$HOME/.gosuv/log/`

Edit config file(default located in `$HOME/.gosuv/programs.yml`) and run `gosuv reload` will take effect immediately.

## Design
HTTP is follow the RESTFul guide.

Get or Update program

`<GET|PUT> /api/programs/:name`

Add new program

`POST /api/programs`

Del program

`DELETE /api/programs/:name`

## State
Only 4 states. [ref](http://supervisord.org/subprocess.html#process-states)

![states](docs/states.png)

## Notification
Configuration example

```yaml
- demo-program:
  command: ...
  notifications:
    pushover:
      api_key: [token]
      users:
      - [user1]
      - [user2]
```

Now only support [pushover](https://pushover.net/api), and only status change to fatal will get notified.

## Integrate with github (todo)
This is feature that will helps update your deployment environment once your updated in the github.

This part is set in the `programs.yml`, take look the example

```yml
- demo-program:
  command: python app.py
  directory: /opt/demo
  webhook:
    github:
      secret: 123456
      command: git pull origin master
```

## Alternative
- Go implementation of supervisor <https://github.com/ochinchina/supervisord>

## LICENSE
[MIT](LICENSE)
