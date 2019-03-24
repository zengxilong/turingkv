package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/jessevdk/go-flags"
	"github.com/lestrrat/go-file-rotatelogs"
	"github.com/turingkv/raft-kv/src/node"
	"github.com/turingkv/raft-kv/src/server"
	"github.com/turingkv/raft-kv/src/utils"
	"os"
	"strconv"
	"strings"
	"time"
)

type Opts struct {
	BindAddress string `long:"bind" env:"BIND" default:"127.0.0.1:3000" description:"ip:port to bind for a node"`
	JoinAddress string `long:"join" env:"JOIN" default:"" description:"ip:port to join for a node"`
	ApiPort     string `long:"api_port" env:"API_PORT" default:":8080" description:":port for a api port"`
	Bootstrap   bool   `long:"bootstrap" env:"BOOTSTRAP" description:"bootstrap a cluster"`
	DataDir     string `long:"data_dir" env:"DATA_DIR" default:"/tmp/data/" description:"Where to store system data"`
	GroupId     int    `long:"group_id" env:"GROUP_ID" default:"0" description:"Raft Group Id"`
	ZkAddress   string `long:"zk_address" env:"ZK_ADDRESS" default:"127.0.0.1:2181" description:"zkServerAddress"`
	LogPath     string `long:"log_path" env:"LOG_PATH" default:"logs/turing-kv.log" description:"logPath"`
}

func main() {
	//参数解析
	var opts Opts
	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.ParseArgs(os.Args[1:]); err != nil {
		log.Panicln(err)
	}

	//日志配置
	if logf, err := rotatelogs.New(
		opts.LogPath+".%Y%m%d",
		rotatelogs.WithMaxAge(time.Hour*24*30),
		rotatelogs.WithRotationTime(time.Hour*24),
	); err != nil {
		log.WithError(err).Error("create rotatelogs, use default io.writer instead")
	} else {
		log.SetOutput(logf)
	}

	log.Infof("'%s' is used to store files of the node", opts.DataDir)

	config := node.Config{
		BindAddress:    opts.BindAddress,
		NodeIdentifier: opts.BindAddress,
		JoinAddress:    opts.JoinAddress,
		DataDir:        opts.DataDir,
		Bootstrap:      opts.Bootstrap,
		ApiPort:        opts.ApiPort,
		GroupId:        opts.GroupId,
	}

	storage, err := node.NewRStorage(&config)
	if err != nil {
		log.Panic(err)
	}

	// 服务注册到zk
	zkServers := strings.Split(opts.ZkAddress, ",")

	client, err := utils.NewClient(zkServers, "/api", 10)
	if err != nil {
		log.Panic(err)
	}

	defer client.Close()

	port, err := strconv.Atoi(opts.ApiPort)
	if err != nil {
		log.Panic(err)
	}

	node_ := &utils.ServiceNode{"group_" + strconv.Itoa(opts.GroupId), strings.Split(opts.BindAddress, ":")[0], port}
	//向zk注册服务信息
	if err := client.Register(node_); err != nil {
		log.Error(err)
	}

	msg := fmt.Sprintf("[INFO] Started node=%s", storage.RaftNode)
	log.Info(msg)

	go printStatus(storage)

	if config.JoinAddress != "" {
		for 1 == 1 {
			time.Sleep(time.Second * 1)
			err := storage.JoinCluster(config.JoinAddress)
			if err != nil {
				log.Info("Can't join the cluster: %+v", err)
			} else {
				break
			}
		}
	}

	// Start an HTTP server
	server.RunHTTPServer(storage, config.ApiPort)
}

func printStatus(s *node.RStorage) {
	for {
		log.Infof("state=%s leader=%s", s.RaftNode.State(), s.RaftNode.Leader())
		time.Sleep(time.Second * 2)
	}
}
