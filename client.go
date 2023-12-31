package main

import (
	"bufio"
	b64 "encoding/base64"
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"strings"
)

type Client struct {
	active              bool
	conn                net.Conn
	reader              *bufio.Reader
	lastTableDefinition TableDefinition
	buffer              []byte
	pointer             int
	mode                string
	tables              map[string]Table
	roomTable           string
	skip                bool
}

func (client *Client) sendHeartBeat() {
	client.conn.Write([]byte{CLASS_CONTROL, HEARTBEAT})
	//time.Sleep(3 * time.Second)
	//client.sendHeartBeat()
}

func (client *Client) sendStatus(remoteId string) {
	client.conn.Write([]byte(SUCCEEDED + "\n"))
}

func (client *Client) initConnection(mode string) {
	client.mode = mode
	client.roomTable = service_vwr_room_table
	message, err := client.reader.ReadString('\n')
	if err != nil {
		client.conn.Close()
		return
	}
	log.Printf("Message incoming: %s\n", string(message))
	if matches := matchString(`^HAProxyS\s+(\d+(\.\d+)?)`, string(message)); len(matches) > 0 {
		if matches[1] != "2.1" {
			client.conn.Write([]byte(BAD_VERSION + "\n"))
		}
		remoteId, err := client.reader.ReadString('\n')
		if err != nil {
			client.conn.Close()
			return
		}
		if remoteId != "lineq\n" {
			client.conn.Write([]byte(REMOTE_ID_MISMATCH + "\n"))
			client.conn.Close()
			return
		}

		peerInfo, _ := client.reader.ReadString('\n')
		if matchesPattern(`.+\s+\d+\s+\d+`, peerInfo) {
			client.sendStatus(remoteId)
			//go client.sendHeartBeat()
			client.tables = make(map[string]Table)
			auto_sync := false
			if auto_sync {
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_REQUEST})
			}
			client.handleRequests()
		}
	} else {
		client.conn.Write([]byte(PROTOCOL_ERROR + "\n"))
	}
	client.conn.Close()
	return
}

func (client *Client) readUpdateAck() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	end := client.pointer + length
	log.Println("End ", end)

	client.pointer += consumed

	consumed, stickTableId, _ := decode(client.buffer[client.pointer:])
	log.Println("stick table id ", stickTableId, consumed)
	client.pointer += consumed

	updateId := binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4])
	log.Println("update id ", updateId)
	client.pointer += 4
}

func (client *Client) readEntryUpdate() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}
	end := client.pointer + length
	log.Println("End ", end)

	client.pointer += consumed

	updateId := binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4])
	client.pointer += 4

	if client.skip {
		client.skip = false
		client.pointer += (end - client.pointer)
		client.sendUpdateAck(tables[client.roomTable].definition, updateId)
		return
	}

	tableDefinition := client.lastTableDefinition

	var keyType int
	var keyValue interface{}

	switch tableDefinition.KeyType {
	case IPv4:
		keyType = IPv4
		keyValue = client.buffer[client.pointer : client.pointer+tableDefinition.KeyLen]
		client.pointer += tableDefinition.KeyLen
	case STRING:
		consumed, keyLen, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		keyType = STRING
		keyValue = string(client.buffer[client.pointer : client.pointer+keyLen])
		client.pointer += keyLen
	case SINT:
		keyType = SINT
		keyValue = int32(binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4]))
		client.pointer += 4
	default:
		log.Println("error key")
		return
	}

	values := make(map[int][]int)
	for i := 0; i < len(tableDefinition.DataTypes); i++ {
		log.Println("DataType[...]", tableDefinition.DataTypes[i])
		switch tableDefinition.DataTypes[i] {
		case SERVER_ID:
			consumed, server_id, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], server_id)
			client.pointer += consumed

			consumed, x, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], x)
			client.pointer += consumed

			consumed, y, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], y)
			client.pointer += consumed

			consumed, nameLen, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], nameLen)
			client.pointer += consumed

			for i := 0; i < nameLen; i++ {
				consumed, char, _ := decode(client.buffer[client.pointer:])
				values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], char)
				client.pointer += consumed
			}
		case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
			consumed, number, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = []int{number}
			client.pointer += consumed
		case HTTP_REQ_RATE:
			consumed, curr_tick, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], curr_tick)
			client.pointer += consumed
			log.Println("cur tick", curr_tick)

			consumed, curr_ctr, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], curr_ctr)
			client.pointer += consumed
			log.Println("req rate", curr_ctr)

			consumed, prev_ctr, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], prev_ctr)
			client.pointer += consumed
			log.Println("prev ctr", prev_ctr)
		case BYTES_IN_CNT, BYTES_OUT_CNT:
			consumed, number, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = []int{number}
			client.pointer += consumed
		default:
			log.Println("error values")
		}
	}

	updateEntry := EntryUpdate{
		UpdateID: updateId,
		Expiry:   0,
		KeyType:  keyType,
		KeyValue: keyValue,
		Values:   values,
	}

	log.Printf("update id %v\n", updateEntry.UpdateID)
	log.Printf("expiry %v\n", updateEntry.Expiry)
	log.Printf("keyType %v\n", updateEntry.KeyType)
	log.Printf("keyValue %v\n", updateEntry.KeyValue)
	log.Printf("values %v\n", updateEntry.Values)

	log.Println("Pointer ", client.pointer)
	log.Println("End ", end)
	keyEnc := client.updateTable(updateEntry)
	client.sendUpdateAck(client.lastTableDefinition, updateId)

	if client.mode == "agg" || client.mode == "vwr" {
		client.updatePeers(client.lastTableDefinition, updateEntry.KeyType, updateEntry.KeyValue, keyEnc)
	}
	sendTableUpdate(tableDefinition.Name, keyEnc)
}

func (client *Client) sendUpdateAck(tableDefinition TableDefinition, uId uint32) {
	message := make([]byte, 0)

	header := []byte{CLASS_UPDATE, UPDATE_ACK}
	encodedTableId := encode(tableDefinition.StickTableID)

	updateId := make([]byte, 4)
	binary.BigEndian.PutUint32(updateId, uId)

	data := append(encodedTableId, updateId...)
	length := encode(len(data))
	message = append(header, length...)
	message = append(message, data...)
	client.conn.Write(message)
}

func (client *Client) readTableDefinition() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}
	end := client.pointer + length

	client.pointer += consumed

	consumed, stickTableId, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	log.Println("stick consumed ", consumed)

	consumed, nameLength, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	log.Printf("name length %v\n", nameLength)

	name := string(client.buffer[client.pointer : client.pointer+nameLength])
	client.pointer += nameLength

	if client.mode == "vwr" && name == client.roomTable {
		client.pointer += (end - client.pointer)
		client.skip = true
		return
	}

	consumed, keyType, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	switch keyType {
	case SINT, IPv4, IPv6, STRING, BINARY:
		log.Printf("well\n")
	default:
		log.Printf("Incorrect key type %v\n", keyType)
		return
	}

	consumed, keyLen, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	consumed, dataType, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	log.Println("dataType consumed ", consumed)
	log.Println("dataType ", dataType)

	consumed, expiry, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	frequency := [][]int{}
	for {
		i := 0

		if client.pointer >= end {
			break
		}

		var freq []int

		consumed, freq_cnt, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		freq = append(freq, freq_cnt)

		consumed, freq_cnt_prd, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		freq = append(freq, freq_cnt_prd)

		log.Printf("counter %v %v\n", freq[0], freq[1])

		frequency = append(frequency, freq)

		i++
	}

	types := [19]int{SERVER_ID, GPT0, GPC0, GPC0_RATE, CONN_CNT, CONN_RATE, CONN_CUR, SESS_CNT, SESS_RATE, HTTP_REQ_CNT,
		HTTP_REQ_RATE, HTTP_ERR_CNT, HTTP_ERR_RATE, BYTES_IN_CNT, BYTES_IN_RATE, BYTES_OUT_CNT, BYTES_OUT_RATE, GPC1, GPC1_RATE}

	dTypes := []int{}

	for i := 0; i < len(types); i++ {
		if ((dataType >> types[i]) & 1) != 0 {
			dTypes = append(dTypes, types[i])
		}
	}

	tableDefinition := TableDefinition{
		StickTableID: stickTableId,
		Name:         name,
		KeyType:      keyType,
		KeyLen:       keyLen,
		DataTypes:    dTypes,
		Expiry:       expiry,
		Frequency:    frequency,
	}

	log.Println("StickTableId ", tableDefinition.StickTableID)
	log.Println("Name ", tableDefinition.Name)
	log.Println("KeyType ", tableDefinition.KeyType)
	log.Println("KeyLen ", tableDefinition.KeyLen)
	log.Println("DataTypes ", tableDefinition.DataTypes)
	log.Println("Expiry ", tableDefinition.Expiry)
	log.Println("Frequency ", tableDefinition.Frequency)

	client.lastTableDefinition = tableDefinition

	if _, exists := client.tables[name]; !exists {
		table := Table{
			localUpdateId: 0,
			definition:    tableDefinition,
		}
		table.entries = make(map[string]Entry)
		client.tables[name] = table
	}

	return
}

func (client *Client) updateTable(entryUpdate EntryUpdate) string {
	tableDefinition := client.lastTableDefinition

	name := tableDefinition.Name
	entry := Entry{
		Key:    entryUpdate.KeyValue,
		Values: entryUpdate.Values,
	}

	table := client.tables[name]

	var key []byte
	switch v := entry.Key.(type) {
	case string:
		key = []byte(v)
	case int32:
		key = s32tob(v)
	case []byte:
		key = v
	default:
		log.Println("Unsupported type")
	}

	jsonKey, _ := json.Marshal(&key)
	keyEnc := b64.StdEncoding.EncodeToString(jsonKey)

	table.entries[keyEnc] = entry

	client.tables[name] = table

	if _, exists := tables[name]; !exists {
		tmp := Table{
			localUpdateId: 0,
			definition:    tableDefinition,
		}
		tmp.entries = make(map[string]Entry)
		tables[name] = tmp
	}

	if client.mode == "agg" {
		tables[name] = table
	} else if client.mode == "vwr" {
		if name == service_vwr_user_table {
			parts := strings.Split(string(key), "@")
			domainPath := parts[1]
			curStat := entry.Values[GPC1][0]
			if curStat == 1 {
				prevEntry, exists := tables[name].entries[keyEnc]
				if !exists || (prevEntry.Values[GPC1][0] == 0) {
					var roomKey []byte = []byte(domainPath)
					roomJson, _ := json.Marshal(&roomKey)
					roomEnc := b64.StdEncoding.EncodeToString(roomJson)

					tables[name].entries[keyEnc] = entry
					tables[client.roomTable].entries[roomEnc].Values[GPC0][0] -= 1
					client.updatePeers(tables[client.roomTable].definition, tables[client.roomTable].definition.KeyType, domainPath, roomEnc)
					cache.Set(keyEnc, []byte(domainPath))
					sendTableUpdate(client.roomTable, roomEnc)
				} else {
					cache.Set(keyEnc, []byte(domainPath))
				}
			} else {
				if _, exists := tables[name].entries[keyEnc]; !exists {
					tables[name].entries[keyEnc] = entry
					sortedEntries[domainPath] = append(sortedEntries[domainPath], keyEnc)
				}
			}
		}
	} else if client.mode == "acc" {
		globTable := tables[name]
		if globTable.entries == nil {
			globTable.entries = make(map[string]Entry)
		}

		globEntry := Entry{
			Key: entryUpdate.KeyValue,
		}

		for i := 0; i < len(tableDefinition.DataTypes); i++ {
			dataType := tableDefinition.DataTypes[i]
			switch dataType {
			case SERVER_ID:
				globEntry.Values[dataType] = make([]int, 0)
			case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
				globEntry.Values[dataType] = make([]int, 1)
			case HTTP_REQ_RATE:
				globEntry.Values[dataType] = make([]int, 3)
			case BYTES_IN_CNT, BYTES_OUT_CNT:
				globEntry.Values[dataType] = make([]int, 1)
			default:
				log.Println("error values")
			}
		}

		for i := 0; i < len(tableDefinition.DataTypes); i++ {
			for i := 0; i < len(peers); i++ {
				if !peers[i].active {
					continue
				}
				if locTable, exists := peers[i].tables[name]; exists {
					if locEnt, exists := locTable.entries[keyEnc]; exists {
						dType := tableDefinition.DataTypes[i]
						switch dType {
						case SERVER_ID:
						case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
						case HTTP_REQ_RATE:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
							globEntry.Values[dType][1] += locEnt.Values[dType][1]
							globEntry.Values[dType][2] += locEnt.Values[dType][2]
						case BYTES_IN_CNT, BYTES_OUT_CNT:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
						default:
							log.Println("error values")
						}
					}
				}
			}
		}

		globTable.entries[keyEnc] = globEntry
		tables[name] = globTable
	}
	return keyEnc
}

func (client *Client) createTableDefinition(tableDefinition TableDefinition) []byte {
	message := make([]byte, 0)
	stickTableId := encode(tableDefinition.StickTableID)
	tableNameLen := encode(len(tableDefinition.Name))
	message = append(stickTableId, tableNameLen...)
	name := []byte(tableDefinition.Name)
	message = append(message, name...)
	keyType := encode(tableDefinition.KeyType)
	message = append(message, keyType...)
	keyLen := encode(tableDefinition.KeyLen)
	message = append(message, keyLen...)

	dataType := 0

	for i := 0; i < len(tableDefinition.DataTypes); i++ {
		dataType = dataType | (1 << tableDefinition.DataTypes[i])
	}

	dataTypeBitFieald := encode(dataType)
	message = append(message, dataTypeBitFieald...)
	expiry := encode(tableDefinition.Expiry)
	message = append(message, expiry...)

	for i := 0; i < len(tableDefinition.Frequency); i++ {
		freq_cnt := encode(tableDefinition.Frequency[i][0])
		message = append(message, freq_cnt...)
		freq_cnt_prd := encode(tableDefinition.Frequency[i][1])
		message = append(message, freq_cnt_prd...)
	}
	return message
}

func (client *Client) createEntryUpdate(tableDef TableDefinition, keyType int, keyValue interface{}, keyEnc string) []byte {
	message := make([]byte, 0)
	tableName := tableDef.Name
	table := tables[tableName]
	table.localUpdateId += 1
	entry := table.entries[keyEnc]

	tables[tableName] = table
	localUpdateId := make([]byte, 4)
	binary.BigEndian.PutUint32(localUpdateId, table.localUpdateId)
	message = append(message, localUpdateId...)

	switch keyType {
	case SINT:
		if value, ok := keyValue.(int32); ok {
			val := uint32(value)
			result := make([]byte, 4)
			binary.BigEndian.PutUint32(result, val)
			message = append(message, result...)
		} else {
			log.Println("Conversion to int32 failed.")
			return nil
		}
	case IPv4:
		log.Println("IPv4")
		if value, ok := keyValue.([]byte); ok {
			message = append(message, value...)

		} else {
			log.Println("Conversion to []byte failed.")
			return nil
		}
	case IPv6:
	case STRING:
		if value, ok := keyValue.(string); ok {
			message = append(message, encode(len(value))...)
			message = append(message, value...)
		}
	case BINARY:
	default:
		log.Printf("Incorrect key type %v\n", keyType)
		return nil
	}

	for i := 0; i < len(tableDef.DataTypes); i++ {
		dataType := tableDef.DataTypes[i]
		switch dataType {
		case SERVER_ID:
			message = append(message, encode(entry.Values[dataType][0])...)
			message = append(message, encode(entry.Values[dataType][1])...)
			message = append(message, encode(entry.Values[dataType][2])...)
			message = append(message, encode(entry.Values[dataType][3])...)

			for i := 0; i < entry.Values[dataType][3]; i++ {
				message = append(message, encode(entry.Values[dataType][4+i])...)
			}
		case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
			message = append(message, encode(entry.Values[dataType][0])...)
		case HTTP_REQ_RATE:
			cur_tick := encode(entry.Values[dataType][0])
			log.Println(entry.Values[dataType][0])
			message = append(message, cur_tick...)

			cur_ctr := encode(entry.Values[dataType][1])
			log.Println(entry.Values[dataType][1])
			message = append(message, cur_ctr...)

			prev_ctr := encode(entry.Values[dataType][2])
			log.Println(entry.Values[dataType][2])
			message = append(message, prev_ctr...)
		case BYTES_IN_CNT, BYTES_OUT_CNT:
		default:
			log.Println("unknown type")
		}
	}
	return message
}

func (client *Client) updatePeer() {
	for key, value := range tables {
		keyType := tables[key].definition.KeyType
		tableDef := client.createTableDefinition(tables[key].definition)
		for keyEnc, entry := range value.entries {
			keyValue := entry.Key
			entryDef := client.createEntryUpdate(tables[key].definition, keyType, keyValue, keyEnc)
			client.sendUpdate(tableDef, entryDef, true)
		}
	}
}

func (client *Client) sendUpdate(tableDef []byte, entryDef []byte, local bool) {
	message := make([]byte, 0)

	tableHeader := []byte{CLASS_UPDATE, STICK_TABLE_DEFINITION}
	tableLength := encode(len(tableDef))
	message = append(tableHeader, tableLength...)
	message = append(message, tableDef...)

	entryHeader := []byte{CLASS_UPDATE, ENTRY_UPDATE}
	entryLength := encode(len(entryDef))
	message = append(message, entryHeader...)
	message = append(message, entryLength...)
	message = append(message, entryDef...)

	if local {
		client.conn.Write(message)
	} else {
		for i := 0; i < len(peers); i++ {
			if peers[i].active {
				peers[i].conn.Write(message)
			}
		}
	}
}

func (client *Client) updatePeers(table TableDefinition, keyType int, keyValue interface{}, keyEnc string) {
	tableDef := client.createTableDefinition(table)
	entryDef := client.createEntryUpdate(table, keyType, keyValue, keyEnc)

	client.sendUpdate(tableDef, entryDef, false)
}

func (client *Client) close() {
	client.active = false
	client.conn.Close()
}

func (client *Client) handleRequests() {
	defer client.close()
	client.buffer = make([]byte, 0)
	client.pointer = 0
	for {
		if client.pointer >= len(client.buffer) {
			tmp := make([]byte, 128)
			n, err := client.reader.Read(tmp)
			if err != nil {
				client.conn.Close()
				return
			}
			client.buffer = tmp[:n]
			client.pointer = 0
		} else if len(client.buffer) == 1 {
			tmp := make([]byte, 128)
			n, err := client.reader.Read(tmp)
			if err != nil {
				client.conn.Close()
				return
			}
			client.buffer = append(client.buffer, tmp[:n]...)
		}

		reqClass := client.buffer[client.pointer]
		client.pointer += 1

		switch reqClass {
		case CLASS_CONTROL:
			log.Println("control class")
			classType := client.buffer[client.pointer]
			client.pointer += 1
			switch classType {
			case HEARTBEAT:
				log.Println("heartbeat")
				client.sendHeartBeat()
			case SYNCHRONIZATION_REQUEST:
				log.Println("synchronization request")
				client.updatePeer()
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_FINISHED})
			case SYNCHRONIZATION_PARTIAL:
				log.Println("synchronization partial")
				//client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_FINISHED})
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_CONFIRMED})
			case SYNCHRONIZATION_CONFIRMED:
				log.Println("synchronization confirmed")
			}
		case CLASS_ERROR:
			log.Println("error class")
			classType := client.buffer[client.pointer]
			client.pointer += 1
			if classType == 0 {
				log.Println("protocol error")
			} else {
				log.Println("size limit error")
			}
		case CLASS_UPDATE:
			log.Println("stick-table updates class")
			classType := client.buffer[client.pointer]
			client.pointer += 1

			switch classType {
			case ENTRY_UPDATE:
				log.Println("entry update")
				client.readEntryUpdate()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			case INCREMENTAL_ENTRY_UPDATE:
				log.Println("incremental entry update")
			case STICK_TABLE_DEFINITION:
				log.Println("stick table definition")
				client.readTableDefinition()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			case STICK_TABLE_SWITCH:
				log.Println("stick table switch")
			case UPDATE_ACK:
				log.Println("update message acknowledgement")
				client.readUpdateAck()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			}
		case CLASS_RESERVED:
			log.Println("reserved class")
		default:
			log.Println("class does not implemented ", reqClass)
		}
	}
}
