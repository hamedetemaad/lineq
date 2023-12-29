package main

import (
	"context"
	"log"
	"time"

	b64 "encoding/base64"
	"encoding/binary"
	"encoding/json"

	"github.com/allegro/bigcache/v3"
)

var cache *bigcache.BigCache

func initCache() {
	vwr_total_users := 100000

	onRemove := func(key string, entry []byte) {
		usersTable := string(entry)

		delete(tables[service_vwr_user_table].entries, key)
		if len(sortedEntries[usersTable]) > 0 {
			newKey := sortedEntries[usersTable][0]
			tables[service_vwr_user_table].entries[newKey].Values[GPC1][0] = 1
			sortedEntries[usersTable] = sortedEntries[usersTable][1:]
			tableDef := tables[service_vwr_user_table].definition
			keyValue := tables[service_vwr_user_table].entries[newKey].Key
			updateClients(tableDef, newKey, keyValue)
			cache.Set(newKey, entry)
			sendTableUpdate(service_vwr_user_table, newKey)
			broadcast()
		} else {
			var roomKey []byte = entry
			roomJson, _ := json.Marshal(&roomKey)
			roomEnc := b64.StdEncoding.EncodeToString(roomJson)
			curVal := tables[service_vwr_room_table].entries[roomEnc].Values[GPC0][0]

			if curVal < routes[usersTable].TOTAL_ACTIVE_USERS {
				tables[service_vwr_room_table].entries[roomEnc].Values[GPC0][0] += 1
				tableDef := tables[service_vwr_room_table].definition

				updateClients(tableDef, roomEnc, usersTable)
				sendTableUpdate(service_vwr_room_table, roomEnc)
			}
		}
	}

	config := bigcache.Config{
		// number of shards (must be a power of 2)
		Shards: 1024,

		// time after which entry can be evicted
		LifeWindow: time.Duration(service_vwr_session_duration) * time.Minute,

		// Interval between removing expired entries (clean up).
		// If set to <= 0 then no action is performed.
		// Setting to < 1 second is counterproductive — bigcache has a one second resolution.
		//CleanWindow: 5 * time.Minute,
		CleanWindow: time.Duration(1) * time.Second,

		// rps * lifeWindow, used only in initial memory allocation
		MaxEntriesInWindow: vwr_total_users,

		// max entry size in bytes, used only in initial memory allocation
		MaxEntrySize: 64,

		// prints information about additional memory allocation
		Verbose: true,

		// cache will not allocate more memory than this limit, value in MB
		// if value is reached then the oldest entries can be overridden for the new ones
		// 0 value means no size limit
		HardMaxCacheSize: vwr_total_users * (64 / 1024 / 1024),

		// callback fired when the oldest entry is removed because of its expiration time or no space left
		// for the new entry, or because delete was called. A bitmask representing the reason will be returned.
		// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
		OnRemove: onRemove,

		// OnRemoveWithReason is a callback fired when the oldest entry is removed because of its expiration time or no space left
		// for the new entry, or because delete was called. A constant representing the reason will be passed through.
		// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
		// Ignored if OnRemove is specified.
		OnRemoveWithReason: nil,
	}

	var initErr error
	cache, initErr = bigcache.New(context.Background(), config)
	if initErr != nil {
		log.Fatal(initErr)
	}
}

func createTableDefinition(tableDefinition TableDefinition) []byte {
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

func createEntryUpdate(tableDef TableDefinition, keyType int, keyValue interface{}, keyEnc string) []byte {
	message := make([]byte, 0)
	tableName := tableDef.Name
	table := tables[tableName]
	table.localUpdateId += 1

	tables[tableName] = table

	entry := table.entries[keyEnc]

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

func updateClients(tdef TableDefinition, keyEnc string, keyValue interface{}) {
	tableDef := createTableDefinition(tdef)
	entryDef := createEntryUpdate(tdef, tdef.KeyType, keyValue, keyEnc)
	for i := 0; i < len(peers); i++ {
		if peers[i].active {
			peers[i].sendUpdate(tableDef, entryDef, true)
		}
	}
}
