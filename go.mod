module gosuv

go 1.16

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/axgle/mahonia v0.0.0-20180208002826-3358181d7394 // indirect
	github.com/axgle/pinyin v0.0.0-20180208003132-d1557e083be4
	github.com/bluele/gcache v0.0.2
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/glycerine/goconvey v0.0.0-20190410193231-58a59202ab31 // indirect
	github.com/glycerine/rbuf v0.0.0-20190314090850-75b78581bebe
	github.com/go-yaml/yaml v2.1.0+incompatible
	github.com/goji/httpauth v0.0.0-20160601135302-2da839ab0f4d
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/hpcloud/tail v1.0.0
	github.com/imroc/req v0.3.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/lunny/dingtalk_webhook v0.0.0-20171025031554-e3534c89ef96
	github.com/mitchellh/go-ps v1.0.0
	github.com/panjf2000/gnet v1.5.3
	github.com/qiniu v1.11.5
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546
	github.com/smartystreets/goconvey v1.6.4
	github.com/soopsio/gosuv v0.0.0-20180126202227-0d2be4371381
	github.com/soopsio/kexec v0.0.0-20160908020525-863094f94c7f
	github.com/thinkeridea/go-extend v1.3.2
	github.com/urfave/cli v1.22.5
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.2.8

)

replace github.com/qiniu v1.11.5 => github.com/qiniu/x v1.11.5
