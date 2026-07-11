package main

import (
	"github.com/pythonwithsean/Yui/yui"
)

func main() {
	s := yui.NewServer()

	s.Get("/", func(req *yui.Request, res *yui.Response) {
		res.SetHeader("Content-Type", "text/html")
		res.Status(200).Send("Sean")
	})

	s.Get("/home", func(req *yui.Request, res *yui.Response) {
		res.Status(200).Send("Daniel")
	})

	s.ListenAndServe("localhost", ":8000")
}
