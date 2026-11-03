package main

import (
	"log"
	"net/http"
	"runtime"
)

func handleHelp(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Help"
	data := struct {
		HeaderData
		GoVersion string
		GoOS      string
		GoArch    string
	}{
		HeaderData: hd,
		GoVersion:  runtime.Version(),
		GoOS:       runtime.GOOS,
		GoArch:     runtime.GOARCH,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "help.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}
