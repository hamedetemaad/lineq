package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"strconv"

	b64 "encoding/base64"
	"encoding/json"
)

var peers []*Client
var sortedEntries []string
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
	service_vwr_session_duration := getenv("SERVICE_VWR_SESSION_DURATION", DEFAULT_VWR_SESSION_DURATION)
	service_vwr_total_users := getenv("SERVICE_VWR_TOTAL_ACTIVE_USERS", DEFAULT_VWR_TOTAL_USERS)
	service_vwr_room_table := getenv("SERVICE_VWR_ROOM_TABLE", DEFAULT_VWR_ROOM_TABLE)
	service_vwr_users_table := getenv("SERVICE_VWR_USERS_TABLE", DEFAULT_VWR_USERS_TABLE)
	vwr_session_duration, _ := strconv.Atoi(service_vwr_session_duration)
	vwr_total_users, _ := strconv.Atoi(service_vwr_total_users)

	go initWebServer(service_web_host, service_web_port)
	listen, err := net.Listen("tcp", service_tcp_host+":"+service_tcp_port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if service_mode == "vwr" {
		initRoomTable(vwr_total_users, service_vwr_room_table)
		go initCache(vwr_session_duration, vwr_total_users, service_vwr_room_table, service_vwr_users_table)
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
		go client.initConnection(service_name, service_mode, vwr_session_duration, vwr_total_users, service_vwr_room_table, service_vwr_users_table)
	}
}

func initRoomTable(vwr_total_users int, roomName string) {
	frequency := [][]int{}
	dType := []int{GPC0}
	tableDefinition := TableDefinition{
		StickTableID: 777,
		Name:         roomName,
		KeyType:      SINT,
		KeyLen:       4,
		DataTypes:    dType,
		Expiry:       24 * 60 * 60 * 1000,
		Frequency:    frequency,
	}

	roomTable := Table{
		localUpdateId: 0,
		definition:    tableDefinition,
	}

	var key []byte = s32tob(1)

	jsonKey, _ := json.Marshal(&key)
	keyEnc := b64.StdEncoding.EncodeToString(jsonKey)

	roomTable.entries = make(map[string]Entry)
	var entryKey int32 = 1
	roomEntry := Entry{
		Key: entryKey,
	}
	roomEntry.Values = make(map[int][]int)
	roomEntry.Values[GPC0] = []int{vwr_total_users}
	roomTable.entries[keyEnc] = roomEntry
	tables[roomName] = roomTable
	sortedEntries = make([]string, 0)
}
