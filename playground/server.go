package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func deadline() time.Time {
	return time.Now().Add(2 * time.Second)
}

func Read(w http.ResponseWriter, r *http.Request) {
	log.Println("READS, PATH:", r.URL.Path)

	// rc := http.NewResponseController(w)
	// if err := rc.SetReadDeadline(deadline()); err != nil {
	// 	log.Fatal(err)
	// }

	var b bytes.Buffer
	if _, err := b.ReadFrom(r.Body); err != nil {
		msg := fmt.Sprintf("ERROR WHILE READING REQUEST BODY: %v", err)
		log.Println(msg)
		_, _ = w.Write([]byte(msg))
	}
}

func CustomRead(w http.ResponseWriter, r *http.Request) {
	log.Println("CUSTOM_READS, PATH:", r.URL.Path)

	// rc := http.NewResponseController(w)
	// if err := rc.SetReadDeadline(deadline()); err != nil {
	// 	log.Fatal(err)
	// }

	time.Sleep(time.Second)
	for {
		buff := make([]byte, 4096)
		if _, err := r.Body.Read(buff); err != nil {
			if err == io.EOF {
				break
			}
			msg := fmt.Sprintf("ERROR WHILE READING REQUEST BODY: %v", err)
			log.Println(msg)
			_, _ = w.Write([]byte(msg))
			break
		}
		time.Sleep(time.Second) // Simulate slow processing
	}
}

func Write(w http.ResponseWriter, r *http.Request) {
	log.Println("WRITES, PATH:", r.URL.Path)

	// rc := http.NewResponseController(w)
	// if err := rc.SetWriteDeadline(deadline()); err != nil {
	// 	log.Fatal(err)
	// }

	time.Sleep(3 * time.Second) // Simulate long work

	response := bytes.Repeat([]byte("x"), 4096)
	if _, err := w.Write(response); err != nil {
		log.Println("ERROR WHILE WRITING RESPONSE:", err)
	}
}

func CustomWrite(w http.ResponseWriter, r *http.Request) {
	log.Println("CUSTOM_WRITES, PATH:", r.URL.Path)

	rc := http.NewResponseController(w)
	// if err := rc.SetWriteDeadline(deadline()); err != nil {
	// 	log.Fatal(err)
	// }

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	response := bytes.Repeat([]byte("x"), 4096)
	responseSize := 1024
	for i := 0; i < len(response); i += responseSize {
		if _, err := fmt.Fprint(w, string(response[i:i+responseSize])); err != nil {
			log.Println("ERROR WHILE WRITING RESPONSE (CUSTOM):", err)
			return
		}
		if err := rc.Flush(); err != nil {
			log.Println("ERROR WHILE FLUSHING RESPONSE (CUSTOM):", err)
			return
		}
		time.Sleep(time.Second) // Simulate slow writing
	}
}

func main() {
	http.HandleFunc("/read", Read)
	http.HandleFunc("/custom_read", CustomRead)
	http.HandleFunc("/write", Write)
	http.HandleFunc("/custom_write", CustomWrite)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("default handler: ", r.URL.Path)
	})

	log.Println("Server started on :80")
	log.Fatal(http.ListenAndServe(":80", nil))
}
