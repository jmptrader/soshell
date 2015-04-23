/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package main

import (
	"encoding/json"
	"flag"
	"golang.org/x/net/websocket"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"
)

var addr = flag.String("http", ":8080", "http service address")
var addrs = flag.String("https", ":8090", "https service address")
var hostname = flag.String("host", "localhost", "domain or host name")

// packet is an extensible object type transmitted via websocket as JSON.
type packet struct {
	Type string
	Args []string
	Map  map[string]string
}

// client is an extensible type representing a single websocket client.
type client struct {
	ws            *websocket.Conn
	user, address string
}

// checkTLS returns "SECURED" if TLS handshake is complete or "UNSECURED" if not.
func checkTLS(r *http.Request) string {
	if r.TLS != nil && r.TLS.HandshakeComplete {
		return "SECURED"
	}
	return "UNSECURED"
}

// newPacket returns an initialized packet. Any arguments are added to the pack.Args
// and the first arg is used for pack.Type.
func newPacket(args ...string) (pack packet) {
	pack.Map = make(map[string]string)
	if len(args) > 0 {
		if len(args) > 1 {
			pack.Type = args[0]
			pack.Args = append(pack.Args, args[1:]...)
		} else {
			pack.Type = args[0]
		}
	}
	return
}

// readPacket reads a single packet from a websocket.
func (c *client) readPacket() (p packet, e error) {
	e = websocket.JSON.Receive(c.ws, &p)
	return
}

// sendPacket converts a packet to JSON then writes it to the websocket.
func (c *client) sendPacket(pack packet) (e error) {
	if j, e := json.Marshal(pack); e == nil {
		_, e = c.ws.Write(j)
	}
	if e != nil {
		log.Println(e)
	}
	return
}

// listener listens for incoming packets and passes them to the respective handlers.
func (c *client) listener() (e error) {
	for {
		if p, e := c.readPacket(); e == nil && len(p.Args) > 0 {
			if cmd, ok := cmdMap[p.Args[0]]; ok {
				e = cmd.Handler(c, p)
			} else {
				e = c.appendMsg("#msgList", p.Args[0]+": command not found ")
			}
		} else {
			break
		}
		time.Sleep(time.Second)
	}
	return
}

var clientTemplate = template.Must(template.ParseFiles("client.html"))

// cHandler serves the websocket client html to the requesting browser.
func cHandler(w http.ResponseWriter, r *http.Request) {
	type data struct {
		SockUrl, Status string
	}
	sockUrl := "wss://" + *hostname + *addrs + "/sock"
	clientTemplate.Execute(w, data{SockUrl: sockUrl, Status: "HTTP " + checkTLS(r)})
}

// wsHandler handles the incoming websocket connections.
func wsHandler(ws *websocket.Conn) {
	if ws.Config().Origin.String() != "https://"+*hostname+*addrs {
		log.Println("Bad Origin!", ws.Config().Origin)
	} else {
		var c = client{ws: ws, address: ws.Request().RemoteAddr}
		if e := c.appendMsg("#msgList", "SOCKET "+checkTLS(ws.Request())); e == nil {
			defer log.Println(c.address, "disconnected")
			log.Println(c.address, "connected")
			e = c.listener()
			if e != nil && e != io.EOF {
				log.Println(e)
			}
		}
	}
}

func main() {
	flag.Parse()
	http.Handle("/", http.HandlerFunc(cHandler))
	http.Handle("/sock", websocket.Handler(wsHandler))
	http.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
	go func() {
		// cert.pem is ssl.crt + *server.ca.pem
		err := http.ListenAndServeTLS(*addrs, "cert.pem", "key.pem", nil)
		if err != nil {
			log.Fatal("ListenAndServeTLS:", err)
		}
	}()
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}