package main

import (
	"bookstore/pb"
	"context"
	"testing"
)

func TestServer_ListBooks(t *testing.T) {
	// 初始化
	db, _ := NewDB("root:756131502@tcp(localhost:3006)/bookstore_db?charset=utf8mb4&parseTime=True&loc=Local")
	s := server{bs: &bookstore{db: db}}
	// rpc 请求
	req := &pb.ListBooksRequest{
		Shelf: 4,
	}
	res, err := s.ListBooks(context.Background(), req)
	if err != nil {
		t.Fatalf("s.ListBooks failed:%v\n", err)
	}
	t.Logf("next_page_token:%v\n", res.GetNextPageToken())
	for i, book := range res.Book {
		t.Logf("%d: %#v\n", i, book)
	}
}
