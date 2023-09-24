package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"strconv"
)

var peers []*Client

var tables = make(map[string]Table)

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {

	service_tcp_host := getenv("SERVICE_TCP_HOST", DEFAULT_TCP_HOST)
	service_tcp_port := getenv("SERVICE_TCP_PORT", DEFAULT_TCP_PORT)
	service_web_host := getenv("SERVICE_WEB_HOST", DEFAULT_WEB_HOST)
	service_web_port := getenv("SERVICE_WEB_PORT", DEFAULT_WEB_PORT)
	service_mode := getenv("SERVICE_MODE", DEFAULT_MODE)
	service_name := getenv("SERVICE_NAME", DEFAULT_NAME) // agg(Aggregation) or acc(Accumulation)
	auto_sync := getenv("SERVICE_AUTO_SYNC", DEFAULT_AUTO_SYNC)

	go initWebServer(service_web_host, service_web_port)
	listen, err := net.Listen("tcp", service_tcp_host+":"+service_tcp_port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		client := &Client{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}
		peers = append(peers, client)
		sync, _ := strconv.ParseBool(auto_sync)
		go client.initConnection(service_name, service_mode, sync)
	}
}
