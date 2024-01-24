/* index.js */
var W = {};
var arr = new Array();
var flag = false;
$(".btn-stop").attr({"disabled": "disabled"});
$(".btn-start").attr({"disabled": "disabled"});
var vm = new Vue({
    el: "#app",
    data: {
        isConnectionAlive: true,
        log: {
            content: '',
            follow: true,
            line_count: 0,
        },
        config: {
            content: '',
            name: '',
        },
        programs: [],
        slaves: [],
        edit: {
            program: null,
            config: null,
        }
    },
    methods: {
        addNewProgram: function (slave) {
            $("#newProgram").modal({
                show: true,
                backdrop: 'static',
            }).data("slave", slave);
        },
        selectStartProgram: function (slave) {
            console.log(27)
            console.log(arr)
            var that = this;
            requestUrl = "/api/programs/selectStartProgram";
            $.ajax({
                url: requestUrl,
                method: 'post',
                dataType: 'json',
                data: JSON.stringify(arr),
                traditional: true,
                success: function (data) {
                    console.log(data);
                    that.cmdHhx()
                }
            })
        },
        selectStopProgram: function (slave) {
            var that = this;
            console.log(arr)
            requestUrl = "/api/programs/selectStopProgram";
            $.ajax({
                url: requestUrl,
                method: 'post',
                dataType: 'json',
                data: JSON.stringify(arr),
                traditional: true,
                success: function (data) {
                    console.log(data);
                    that.cmdHhx();
                }
            })
        },
        cmdTest: function (name) {
            console.log(name)
        },
        formNewProgram: function () {
            var url = "/api/programs",
                data = $("#formNewProgram").serialize(),
                name = $("#formNewProgram").find("[name=name]").val(),
                disablechars = "./\\";
            if (!name) {
                alert("\"" + name + "\" is empty ")
                return false
            }
            if (disablechars.indexOf(name[0]) != -1) {
                alert("\"" + name + "\" Can't starts with \".\" \"/\" \"\\\"")
                return false
            }
            var slave = $("#newProgram").data("slave");
            if (slave !== undefined && slave !== "") {
                url = "/distributed/" + slave + url;
            }
            $.ajax({
                type: "POST",
                url: url,
                data: data,
                success: function (data) {
                    if (data.status === 0) {
                        $("#newProgram").modal('hide');
                    } else {
                        window.alert(data.error);
                    }
                },
                error: function (err) {
                    alert(err.responseText)
                }
            });
        },
        showEditProgram: function (p, slave) {
            this.edit.program = Object.assign({}, p); // here require polyfill.min.js
            $("#programEdit").data("slave", slave).modal('show');
        },
        editProgram: function () {
            var p = this.edit.program;
            var requestUrl = "/api/programs/" + p.name
            var slave = $("#programEdit").data("slave");
            if (slave !== undefined && slave !== "") {
                requestUrl = "/distributed/" + slave + requestUrl;
            }
            $.ajax({
                url: requestUrl,
                method: "PUT",
                data: JSON.stringify(p),
            }).then(function (ret) {
                $("#programEdit").modal('hide');
            })
        },
        updateBreadcrumb: function () {
            var pathname = decodeURI(location.pathname || "/");
            var parts = pathname.split('/');
            this.breadcrumb = [];
            if (pathname == "/") {
                return this.breadcrumb;
            }
            var i = 2;
            for (; i <= parts.length; i += 1) {
                var name = parts[i - 1];
                var path = parts.slice(0, i).join('/');
                this.breadcrumb.push({
                    name: name + (i == parts.length ? ' /' : ''),
                    path: path
                })
            }
            return this.breadcrumb;
        },
        refresh: function () {
            // ws.send("Hello")
            $.ajax({
                url: "/api/programs",
                success: function (data) {
                    vm.programs = data;
                    Vue.nextTick(function () {
                        $('[data-toggle="tooltip"]').tooltip()
                    })
                }
            });

            $.ajax({
                url: "/distributed/api/programs",
                success: function (data) {
                    vm.slaves = data;
                    Vue.nextTick(function () {
                        $('[data-toggle="tooltip"]').tooltip()
                    })
                }
            });
        },
        /*reload: function () {
            $.ajax({
                url: "/api/reload",
                method: "POST",
                success: function (data) {
                    if (data.status == 0) {
                        alert("reload success");
                    } else {
                        alert(data.value);
                    }
                }
            });
        },
        test: function () {
            console.log("test");
        },*/
        cmdStart: function (name, slave) {
            console.log(name, slave);
            requestUrl = "/api/programs/" + name + "/start";
            if (slave !== undefined && "" !== slave) {
                requestUrl = "/distributed/" + slave + requestUrl;
            }
            $.ajax({
                url: requestUrl,
                method: 'post',
                success: function (data) {
                    console.log(data);
                }
            });
        },
        cmdStop: function (name, slave) {
            requestUrl = "/api/programs/" + name + "/stop";
            if (slave !== undefined && "" !== slave) {
                requestUrl = "/distributed/" + slave + requestUrl;
            }
            $.ajax({
                url: requestUrl,
                method: 'post',
                success: function (data) {
                    console.log(data);
                }
            })
        },
        cmdSelect: function (name, slave) {
            if (arr.indexOf(name) == -1) {
                arr.push(name);
            } else {
                arr.splice(arr.indexOf(name), 1)
            }
            if (arr.length > 0) {
                $(".btn-stop").removeAttr("disabled");//将按钮可用
                $(".btn-start").removeAttr("disabled");//将按钮可用
            } else {
                $(".btn-stop").prop({"disabled": "disabled"});
                $(".btn-start").prop({"disabled": "disabled"});
            }
            console.log(arr)
        },
        cmdRestart: function (name, slave) {
            requestUrl = "/api/programs/" + name + "/restart";
            if (slave !== undefined && "" !== slave) {
                requestUrl = "/distributed/" + slave + requestUrl;
            }
            $.ajax({
                url: requestUrl,
                method: 'post',
                success: function (data) {
                    console.log(data);
                }
            })
        },
        cmdTail: function (name, slave) {
            requestUrl = "/ws/logs/" + name;
            if (slave !== undefined && "" !== slave) {
                requestUrl = "/distributed/" + slave + requestUrl;
            }
            var that = this;
            if (W.wsLog) {
                W.wsLog.close()
            }
            W.wsLog = newWebsocket(requestUrl, {
                onopen: function (evt) {
                    that.log.content = "";
                    that.log.line_count = 0;
                },
                onmessage: function (evt) {
                    console.log("on")
                    that.log.content += evt.data.replace(/\033\[[0-9;]*m/g, "");
                    that.log.line_count = $.trim(that.log.content).split(/\r\n|\r|\n/).length;
                    // console.log("data" + evt.data)
                    // console.log("line_count" + that.log.line_count)
                    if (that.log.follow) {
                        var pre = $(".realtime-log")[0];
                        setTimeout(function () {
                            pre.scrollTop = pre.scrollHeight - pre.clientHeight;
                        }, 1);
                    }
                }
            });
            this.log.follow = true;
            $("#modalTailf").modal({
                show: true,
                keyboard: true,
            }).on("hide.bs.modal", function (e) {
                W.wsLog.close();
            })
        },
        cmdDelete: function (name, slave) {
            if (!confirm("Confirm delete \"" + name + "\"")) {
                return
            }
            requestUrl = "/api/programs/" + name;
            if (slave !== undefined && "" !== slave) {
                requestUrl = "/distributed/" + slave + requestUrl
            }
            $.ajax({
                url: requestUrl,
                method: 'delete',
                success: function (data) {
                    console.log(data);
                }
            })
        },
        canStop: function (status) {
            switch (status) {
                case "running":
                case "retry wait":
                    return true;
            }
        },
        cmdSingle: function () {
            if (flag) {
                this.cmdHhx();
            } else {
                $("input[class='single']:checkbox").prop('checked', true);
                $("input[class='allIn']:checkbox").prop('checked', true);
                arr = [];
                $("input[class='single']:checked").each(function (i) {
                    arr.push($(this).val())
                })
                flag = true;
                $(".btn-stop").removeAttr("disabled");//将按钮可用
                $(".btn-start").removeAttr("disabled");//将按钮可用
            }
        },
        cmdHhx: function () {
            $("input[class='single']:checkbox").prop('checked', false);
            $("input[class='allIn']:checkbox").prop('checked', false);
            arr = [];
            flag = false;
            $(".btn-stop").attr({"disabled": "disabled"});
            $(".btn-start").attr({"disabled": "disabled"});
        },
        cmdConfig: function (name) {
            this.cmdCommonConfig(name)
            $("#modalConf").modal({
                show: true,
                keyboard: true,
            }).on("hide.bs.modal", function (e) {
            })
        },
        showEditConfig: function (p) {
            if (p.config_path == '') {
                alert("this config_path is empty ")
                return false
            }
            this.cmdCommonConfig(p.name)
            this.edit.program = Object.assign({}, p);
            $("#configEdit").modal('show');
        },
        cmdCommonConfig: function (name) {
            var that = this;
            requestUrl = "/api/config/" + name;
            $.ajax({
                url: requestUrl,
                method: 'get',
                success: function (data) {
                    that.config.content = data;
                    that.config.name = name;
                    // console.log(data);
                }
            });
        },
        editConfig: function () {
            var requestUrl = "/api/config/" + this.config.name
            $.ajax({
                url: requestUrl,
                method: "PUT",
                data: JSON.stringify(this.config.content),
            }).then(function (ret) {
                $("#configEdit").modal('hide');
            })
        },
    }
})

Vue.filter('fromNow', function (value) {
    return moment(value).fromNow();
})

Vue.filter('formatBytes', function (value) {
    var bytes = parseFloat(value);
    if (bytes < 0) return "-";
    else if (bytes < 1024) return bytes + " B";
    else if (bytes < 1048576) return (bytes / 1024).toFixed(0) + " KB";
    else if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + " MB";
    else return (bytes / 1073741824).toFixed(1) + " GB";
})

Vue.filter('colorStatus', function (value) {
    var makeColorText = function (text, color) {
        return "<span class='status' style='background-color:" + color + "'>" + text + "</span>";
    };
    switch (value) {
        case "stopping":
            return makeColorText(value, "#996633");
        case "running":
            return makeColorText(value, "green");
        case "fatal":
            return makeColorText(value, "red");
        default:
            return makeColorText(value, "gray");
    }
});

Vue.directive('disable', function (value) {
    this.el.disabled = !!value
});

$(function () {
    vm.refresh();

    function newEventWatcher() {
        W.events = newWebsocket("/ws/events", {
            onopen: function (evt) {
                vm.isConnectionAlive = true;
            },
            onmessage: function (evt) {
                console.log("response:" + evt.data);
                vm.refresh();
            },
            onclose: function (evt) {
                W.events = null;
                vm.isConnectionAlive = false;
                console.log("Reconnect after 3s");
                setTimeout(newEventWatcher, 3000)
            }
        });
    };

    newEventWatcher();

    // cancel follow log if people want to see the original data
    $(".realtime-log").bind('mousewheel', function (evt) {
        if (evt.originalEvent.wheelDelta >= 0) {
            vm.log.follow = false;
        }
    });
    $('#modalTailf').on('hidden.bs.modal', function () {
        // do something…
        if (W.wsLog) {
            console.log("wsLog closed");
            W.wsLog.close()
        }
    })
});