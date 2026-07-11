package main

import (
	"fmt"

	"github.com/pythonwithsean/Yui/yui"
)

func main() {
	s := yui.NewServer()
	s.Get("/", func(req *yui.Request, res *yui.Response) {
	})
	s.Get("/home", func(req *yui.Request, res *yui.Response) {
		fmt.Println("Hello from /home endpoint")
	})
	s.ListenAndServe("localhost", ":8000")
}
