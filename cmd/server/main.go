package main

import (
	"flag"
	"log"
	"net/http"
	"novel_edit/frontend"
	"novel_edit/internal/workspace"
	"time"
)

func main() {
	root := flag.String("data", "data", "小说数据目录")
	address := flag.String("listen", "127.0.0.1:48120", "监听地址")
	flag.Parse()
	store, err := workspace.NewStore(*root)
	if err != nil {
		log.Fatal(err)
	}
	server := &workspace.Server{Store: store, Assets: frontend.Assets()}
	log.Printf("砚台已启动：http://%s", *address)
	httpServer := &http.Server{Addr: *address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(httpServer.ListenAndServe())
}
