//go:build !linux

package main

import (
	"log"
	"net/http"
)

func handleTerminalPage(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Terminal"
	data := struct {
		HeaderData
	}{
		HeaderData: hd,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "terminal.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "terminal is only available on Linux", http.StatusNotImplemented)
}
