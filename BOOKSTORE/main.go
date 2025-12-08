package main

import (
	"bookstore/pb"
	"context"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"net/http"
	"strings"
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
	l, err := net.Listen("tcp", ":8091")
	if err != nil {
		fmt.Printf("failed to listen, err:%v\n", err)
		return
	}
	s := grpc.NewServer()
	// 注册服务
	pb.RegisterBookstoreServer(s, &srv)
	// 同一个端口分别处理gRPC和http
	// 1. 创建gRPC-Gateway mux
	gwmux := runtime.NewServeMux()
	dops := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := pb.RegisterBookstoreHandlerFromEndpoint(context.Background(), gwmux, "127.0.0.1:8091", dops); err != nil {
		log.Fatalln("Failed to register gwmux:", err)
	}
	// 2. 新建HTTP mux
	mux := http.NewServeMux()
	mux.Handle("/", gwmux)

	// 3. 定义HTTP server配置
	gwServer := &http.Server{
		Addr:    "127.0.0.1:8091",
		Handler: grpcHandlerFunc(s, mux), // 请求的统一入口
	}
	// 4. 启动
	log.Println("Serving on http://127.0.0.1:8091")
	log.Fatalln(gwServer.Serve(l)) // 启动HTTP服务

	/**
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
	*/
}

// grpcHandlerFunc 将gRPC请求和HTTP请求分别调用不同的handler处理
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	}), &http2.Server{})
}
