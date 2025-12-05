package main

import (
	"bookstore/pb"
	"context"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/http"
)

func main() {
	// 连接数据库
	db, err := NewDB("root:756131502@tcp(localhost:3006)/bookstore_db?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		fmt.Printf("连接数据库失败: %v\n", err)
		return
	}
	fmt.Println(db)
	// 创建server
	srv := server{
		bs: &bookstore{db: db},
	}

	// 启动gRPC服务
	l, err := net.Listen("tcp", ":8972")
	if err != nil {
		fmt.Printf("failed to listen, err:%v\n", err)
		return
	}
	s := grpc.NewServer()
	// 注册服务
	pb.RegisterBookstoreServer(s, &srv)
	go func() {
		fmt.Println(s.Serve(l))
	}()
	// 启动gRPC-Gateway
	conn, err := grpc.NewClient(
		"127.0.0.1:8972",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Printf("grpc conn failed, err:%v\n", err)
	}

	// 创建gRPC-Gateway的ServeMux
	gwmux := runtime.NewServeMux()
	pb.RegisterBookstoreHandler(context.Background(), gwmux, conn)
	gwServer := &http.Server{
		Addr:    ":8090",
		Handler: gwmux,
	}
	fmt.Println("grpc-Gateway serve on :8090")
	gwServer.ListenAndServe()
}
