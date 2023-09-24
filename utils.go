package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

func decode(buffer []byte) (int, int, error) {
	if len(buffer) < 1 {
		return 0, 0, errors.New("Insufficient data")
	}

	val := int(buffer[0])
	buffer = buffer[1:]

	if (val & 0xf0) != 0xf0 {
		return 1, val, nil
	}

	for i, b := range buffer {
		val += int(b) << uint(4+7*i)
		if (b & 0x80) == 0 {
			return 2 + i, val, nil
		}
	}

	return 0, 0, errors.New("Insufficient data")
}

func encode(input int) []byte {
	var result []byte

	if input < 0xf0 {
		return []byte{byte(input)}
	}

	result = append(result, byte(input&0xff|0xf0))
	input -= 0xf0
	input = input >> 4

	for input >= 0x80 {
		result = append(result, byte(input&0x7f|0x80))
		input -= 0x80
		input = input >> 7
	}

	result = append(result, byte(input))
	return result
}

func matchString(pattern string, text string) []string {
	regex := regexp.MustCompile(pattern)
	return regex.FindStringSubmatch(text)
}

func matchesPattern(pattern string, text string) bool {
	regex := regexp.MustCompile(pattern)
	return regex.MatchString(text)
}

func itob(val int) []byte {
	r := make([]byte, 4)
	for i := int(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}

func s32tob(val int32) []byte {
	r := make([]byte, 4)
	for i := int32(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}

func u32tob(val uint32) []byte {
	r := make([]byte, 4)
	for i := uint32(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}

func ipToString(data []byte, ipv6 bool) string {
	s := make([]string, len(data))
	for i := range data {
		s[i] = strconv.Itoa(int(data[i]))
	}

	if ipv6 {
		return strings.Join(s, "::")
	} else {
		return strings.Join(s, ".")
	}
}
