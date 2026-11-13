package main

import "unicode"
import "strconv"
import "strings"
import "fmt"


type QueryType int

const (
	Type_Exact QueryType = iota
	Type_LessThan
	Type_GreatThan
	Type_Range
	Type_Error
)

type QueryData struct {
	Type QueryType 
	QueryValueA int64
	QueryValueB int64
}

func parseNumber(str string) (int64, int) {
	i := 0



	for i<len(str) && unicode.IsDigit(rune(str[i])) {
		i+= 1	
	}
	val, err := strconv.ParseInt(str[0:i],10,64)
	if err != nil {
		return -1,-1
	}
	return val,i
}


func parseRangeSeparator(str string) (int) {
	if len(str) < 3 {
		return -1
	}
	if str[0:3] == "..." {
		return 3
	}else {
		return -1
	}
}


func parseRange(str string) QueryData {
	quer_dat := QueryData{Type: Type_Range}
	outIndex := 0
	quer_dat.QueryValueA,outIndex = parseNumber(str)
	if outIndex == -1 {
		fmt.Println("Error in parseRange:LHS")
		quer_dat.Type = Type_Error
		return quer_dat
	}
	outIndex += parseRangeSeparator(str[outIndex:])
	if outIndex == -1 {
		fmt.Println("Error in parseRange:SEP")
		quer_dat.Type = Type_Error
		return quer_dat
	}
	
	quer_dat.QueryValueB,outIndex = parseNumber(str[outIndex:])
	if outIndex == -1 {
		fmt.Println("Error in parseRange:RHS")
		quer_dat.Type = Type_Error
		return quer_dat
	}
	return quer_dat
}


func parseTimestampQuery(str string) QueryData{
	quer := strings.TrimSpace(str)
	quer_dat := QueryData{Type: Type_Error}
	
	switch quer[0] {
	case '<':
		quer_dat.Type = Type_LessThan
		quer_dat.QueryValueA,_ = parseNumber(str[1:])
	case '>':
		quer_dat.Type = Type_GreatThan
		quer_dat.QueryValueA,_ = parseNumber(str[1:])
	case '=':
		quer_dat.Type = Type_Exact
		quer_dat.QueryValueA,_ = parseNumber(str[1:])
	default:
		return parseRange(str)
	}
	return quer_dat
}
