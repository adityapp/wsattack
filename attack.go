package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type attk struct {
	FileToProcess string
	DirToProcess  []os.DirEntry

	generator map[string]ruleGenerator

	wsTarget string
	wsAuth   map[string]string
	ws       *websocket.Conn

	atkTimes int64

	numberOfIterartion int64
}

type ruleGenerator struct {
	rawContent []byte
	rawMap     map[string]interface{}

	ruleRelation map[string]rule

	strTemplate string
	template    *template.Template
}

type rule struct {
	// raw rule
	rawRule string
	isAsIs  bool

	// modified field prefix
	prefixField   string
	retentionTime int64

	// type check
	dataTypeRaw string
	dataType    reflect.Kind
	isUUID      bool
	isUnixtime  bool
	isSlice     bool

	// variant ref
	// unixtime
	unixTimeRef int64

	// variant check
	// element increment
	isIncElSize          bool
	incElSizeRanRangeMin int64
	incElSizeRanRangeMax int64

	// variant check
	// random
	isRan       bool
	ranRangeMin int64
	ranRangeMax int64

	// variant check
	// choose one
	isChooseOne    bool
	chooseOneRange []string

	// variant check
	// sync check
	isSync      bool
	fieldToSync string
}

type generatedResult struct {
	varResult map[string]fieldResult
}

type fieldResult struct {
	isPopulated bool
	incElSize   int64
	ranInt      int64
	chosen      string
	chosenIdx   int

	result      interface{}
	sliceFormat string
}

func NewWsAtk(fileDir []os.DirEntry, fileSeeder, wsTarget string, wsAuth map[string]string, atkTimes int64) (*attk, error) {
	wsAtk := &attk{
		FileToProcess:      fileSeeder,
		DirToProcess:       fileDir,
		wsTarget:           wsTarget,
		wsAuth:             wsAuth,
		generator:          make(map[string]ruleGenerator),
		atkTimes:           atkTimes,
		numberOfIterartion: 10,
	}

	if err := wsAtk.ConnectWs(); err != nil {
		return nil, fmt.Errorf("connect ws %w", err)
	}

	// if err := wsAtk.ReadFiles(); err != nil {
	// 	return nil, fmt.Errorf("file read %w", err)
	// }

	// if err := wsAtk.BeginParsing(); err != nil {
	// 	return nil, fmt.Errorf("rule parsing %w", err)
	// }

	return wsAtk, nil
}

func (a *attk) BeginAttack() error {
	return nil
}

func (a *attk) ConnectWs() error {
	wsHeader := http.Header{}
	for k, v := range a.wsAuth {
		wsHeader[k] = []string{v}
	}

	conn, resp, err := websocket.DefaultDialer.Dial(a.wsTarget, wsHeader)
	if err != nil {
		return fmt.Errorf("dial ws %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("non-OK response %d", resp.StatusCode)
	}

	a.ws = conn
	return nil
}

func (a *attk) ReadFiles() error {
	filenames := []string{}
	for _, aFile := range a.DirToProcess {
		if aFile.IsDir() {
			continue
		}

		filenames = append(filenames, aFile.Name())
	}

	filenames = append(filenames, a.FileToProcess)

	for _, readFile := range filenames {
		fRead, err := os.Open(readFile)
		if err != nil {
			continue
		}

		fContent, err := io.ReadAll(fRead)
		if err != nil {
			fRead.Close()
			continue
		}

		ruleGen := ruleGenerator{ruleRelation: make(map[string]rule)}
		if err := json.Unmarshal(fContent, &ruleGen.rawMap); err != nil {
			fRead.Close()
			continue
		}

		ruleGen.rawContent = fContent
		ruleGen.strTemplate = strings.TrimSpace(string(fContent))

		a.generator[readFile] = ruleGen
		fRead.Close()
	}

	return nil
}

func (a *attk) BeginParsing() error {
	for _, fileToGen := range a.generator {
		if err := fileToGen.parse("", fileToGen.rawMap, ""); err != nil {
			return fmt.Errorf("parsing rule %w", err)
		}

		// fmt.Fprintf(os.Stdout, "template:\n%s\n", fileToGen.strTemplate)

		var err error
		fileToGen.template, err = template.New("").Parse(fileToGen.strTemplate)
		if err != nil {
			return fmt.Errorf("template parsing %w", err)
		}

		for i := int64(0); i < a.atkTimes; i++ {
			res, err := fileToGen.generate()
			if err != nil {
				return fmt.Errorf("generate val %w", err)
			}
			fmt.Fprintf(os.Stdout, "result:\n%s\n", string(res))

		}
	}

	return nil
}

func (rg *ruleGenerator) generateValue() (generatedResult, error) {
	// iterate each rule
	var valueGen = generatedResult{
		varResult: make(map[string]fieldResult),
	}
	for templateName, ruleSet := range rg.ruleRelation {
		// fmt.Fprintf(os.Stdout, "checking field %s\nrule:%+v\n", templateName, ruleSet)
		// init var
		var valGen interface{}
		switch ruleSet.dataType {
		case reflect.String:
			if ruleSet.isSlice {
				valGen = []string{""}
			} else if ruleSet.isUUID {
				valGen = uuid.UUID{}
			} else {
				valGen = ""
			}
		case reflect.Int64:
			if ruleSet.isSlice {
				valGen = []int64{0}
			} else if ruleSet.isUnixtime {
				valGen = time.Time{}.Unix()
			} else {
				valGen = int64(0)
			}
		}

		var resRef fieldResult
		if ruleSet.isSync {
			resRef = valueGen.varResult[ruleSet.prefixField+ruleSet.fieldToSync]
		}

		// check variant
		// it's assume that each variant rule doesn't collide
		// check for slice
		if ruleSet.isSlice && (ruleSet.dataType == reflect.Int64 || ruleSet.dataType == reflect.String) && ruleSet.isIncElSize {
			var sizeEl int64
			if ruleSet.isSync && resRef.isPopulated {
				sizeEl = resRef.incElSize
			} else {
				sizeEl = ruleSet.incElSizeRanRangeMin + rand.Int63n(ruleSet.incElSizeRanRangeMax-ruleSet.incElSizeRanRangeMin)
			}

			var sliceFormat string
			switch ruleSet.dataType {
			case reflect.Int64:
				tempEl := make([]int64, sizeEl)
				tempElStr := make([]string, sizeEl)
				for i := int64(0); i < sizeEl; i++ {
					val := ruleSet.int64Gen()
					tempEl[i] = val
					tempElStr[i] = fmt.Sprintf("%d", val)
				}
				valGen = tempEl
				sliceFormat = "[" + strings.Join(tempElStr, ",") + "]"
			case reflect.String:
				tempEl := make([]string, sizeEl)
				tempElStr := make([]string, sizeEl)
				for i := int64(0); i < sizeEl; i++ {
					val := ruleSet.stringGen()
					tempEl[i] = val
					tempElStr[i] = fmt.Sprintf("\"%s\"", val)
				}
				valGen = tempEl
				sliceFormat = "[" + strings.Join(tempElStr, ",") + "]"
			}

			valueGen.varResult[templateName] = fieldResult{
				isPopulated: true,
				incElSize:   sizeEl,
				result:      valGen,
				sliceFormat: sliceFormat,
			}
			// fmt.Fprintf(os.Stdout, "result 1: %v\n", valGen)
			continue
		}

		// check for int
		if ruleSet.dataType == reflect.Int64 && ruleSet.isRan {
			var ranRes int64
			if ruleSet.isSync && resRef.isPopulated {
				ranRes = resRef.ranInt
			} else {
				ranRes = ruleSet.ranRangeMin + rand.Int63n(ruleSet.ranRangeMax-ruleSet.ranRangeMin)
			}

			if ruleSet.isUnixtime {
				valGen = time.Now().Unix() + ranRes
			} else {
				valGen = ranRes
			}

			valueGen.varResult[templateName] = fieldResult{
				isPopulated: true,
				ranInt:      ranRes,
				result:      valGen,
			}
			// fmt.Fprintf(os.Stdout, "result 2: %v\n", valGen)
			continue
		}

		// check for chosen value
		if ruleSet.isChooseOne {
			var chosenIdx int
			if ruleSet.isSync && resRef.isPopulated {
				chosenIdx = resRef.chosenIdx
			} else {
				chosenIdx = rand.Intn(len(ruleSet.chooseOneRange))
			}
			chosenVal := ruleSet.chooseOneRange[chosenIdx]

			valGen = chosenVal
			valueGen.varResult[templateName] = fieldResult{
				isPopulated: true,
				chosenIdx:   chosenIdx,
				chosen:      chosenVal,
				result:      valGen,
			}
			// fmt.Fprintf(os.Stdout, "result 3: %v\n", valGen)
			continue
		}

		// ran string
		if ruleSet.isUUID {
			valGen = ruleSet.uuidGen()
			valueGen.varResult[templateName] = fieldResult{
				isPopulated: true,
				result:      valGen,
			}
			// fmt.Fprintf(os.Stdout, "result 4: %v\n", valGen)
			continue
		}

		// the rest must be string random
		valGen = ruleSet.stringGen()
		valueGen.varResult[templateName] = fieldResult{
			isPopulated: true,
			result:      valGen,
		}
		// fmt.Fprintf(os.Stdout, "result 5: %v\n", valGen)
	}
	return valueGen, nil
}

func (r *rule) stringGen() string {
	return "abcdefg"
}

func (r *rule) int64Gen() int64 {
	return int64(0)
}

func (r *rule) uuidGen() uuid.UUID {
	return uuid.New()
}

func (r *rule) unixtimeGen() int64 {
	return r.unixTimeRef
}

func (rg *ruleGenerator) generate() ([]byte, error) {
	// gen value
	valRes, err := rg.generateValue()
	if err != nil {
		return nil, fmt.Errorf("generate value %w", err)
	}

	// for idx, val := range valRes.varResult {
	// 	// print generated value
	// 	fmt.Fprintf(os.Stdout, "generated value for field %s is %v\n", idx, val.result)
	// }

	var valueMap = make(map[string]interface{})
	for fieldName, fieldVal := range valRes.varResult {
		if fieldVal.sliceFormat != "" {
			valueMap[fieldName] = fieldVal.sliceFormat
		} else {
			valueMap[fieldName] = fieldVal.result
		}
	}
	buff := bytes.Buffer{}
	err = rg.template.Execute(&buff, valueMap)
	if err != nil {
		return nil, fmt.Errorf("template exec %w", err)
	}

	// fmt.Fprintf(os.Stdout, "check template parsed: %s", buff.String())

	var temp interface{}
	err = json.Unmarshal(buff.Bytes(), &temp)
	if err != nil {
		return nil, fmt.Errorf("template unmarshal %w", err)
	}
	// fmt.Fprintf(os.Stdout, "\ntemplate unmarshal result:\n%v", temp)
	return buff.Bytes(), nil
}

func (rg *ruleGenerator) parse(keyStr string, val interface{}, prefixStr string) error {
	refVal := reflect.TypeOf(val)
	switch refVal.Kind() {
	case reflect.String:
		stringVal, _ := val.(string)
		stringValRef := stringVal
		ruleHolder := rule{rawRule: stringVal}
		if !strings.HasPrefix(stringVal, "rule{{") {
			ruleHolder.isAsIs = true
			break
		}

		// remove prefix rule, '{{'
		// remove suffix '}}'
		stringVal = strings.TrimPrefix(stringVal, "rule{{")
		stringVal = strings.TrimSuffix(stringVal, "}}")
		ruleset := strings.Split(stringVal, ";")
		for _, onerule := range ruleset {
			err := ruleHolder.ruleParsing(0, strings.Split(onerule, ":"), true)
			if err != nil {
				return err
			}
		}

		prefixDelim := ""
		if prefixStr != "" {
			prefixDelim = "__"
		}
		ruleRelationKey := prefixStr + prefixDelim + keyStr
		ruleHolder.prefixField = prefixStr + prefixDelim
		rg.ruleRelation[ruleRelationKey] = ruleHolder

		templateValFormat := "{{.%s}}"
		if ruleHolder.dataType == reflect.String && !ruleHolder.isSlice {
			templateValFormat = "\"{{.%s}}\""
		}

		rg.strTemplate = strings.ReplaceAll(rg.strTemplate, "\""+stringValRef+"\"", fmt.Sprintf(templateValFormat, ruleRelationKey))
		// fmt.Fprintf(os.Stdout, "check rule %+v\n", ruleHolder)
		// fmt.Fprintln(os.Stdout)

	case reflect.Map:
		valMap, _ := val.(map[string]interface{})
		for mapKey, mapParse := range valMap {
			err := rg.parse(mapKey, mapParse, keyStr)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

var (
	ErrEndOfParser           = fmt.Errorf("end of parser")
	ErrSliceWithNoPrecedence = fmt.Errorf("slice with no precedence type")
	ErrFormatMustTypeString  = fmt.Errorf("only type string can be formatted")
	ErrRangeEmptySet         = fmt.Errorf("range options must not be an empty set")
	ErrRangeRanMustBeTwo     = fmt.Errorf("range of random must specify lowest and highest")
	ErrMustBeNumber          = fmt.Errorf("must be integer number")
)

func (r *rule) ruleParsing(idx int, ruleString []string, initParsing bool) error {
	if len(ruleString) == 0 {
		return ErrEndOfParser
	}

	// fmt.Fprintf(os.Stdout, "to parse %s\n", ruleString)

	switch ruleString[0] {
	case "type":
		switch ruleString[1] {
		case "string":
			r.dataTypeRaw = "string"
			r.dataType = reflect.String
		case "uuid":
			r.dataTypeRaw = "string"
			r.dataType = reflect.String
			r.isUUID = true
		case "int64":
			r.dataTypeRaw = "int64"
			r.dataType = reflect.Int64
		case "unixtime":
			r.dataTypeRaw = "int64"
			r.dataType = reflect.Int64
			r.isUnixtime = true
		case "slice":
			// only if accompany with previous valid type
			if r.dataTypeRaw == "" {
				return ErrSliceWithNoPrecedence
			}
			r.isSlice = true
		}
	case "choose_one":
		arrayList := ruleString[1]
		arrayList = strings.TrimPrefix(arrayList, "[")
		arrayList = strings.TrimSuffix(arrayList, "]")
		r.chooseOneRange = strings.Split(arrayList, ",")
		if len(r.chooseOneRange) == 0 {
			return ErrRangeEmptySet
		}
		r.isChooseOne = true
	case "ran":
		ranRange := strings.Split(ruleString[1], "..")
		if len(ranRange) != 2 {
			return ErrRangeRanMustBeTwo
		}

		var err error
		r.ranRangeMin, err = strconv.ParseInt(ranRange[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ran min range %w", ErrMustBeNumber)
		}
		r.ranRangeMax, err = strconv.ParseInt(ranRange[1], 10, 64)
		if err != nil {
			return fmt.Errorf("ran max range %w", ErrMustBeNumber)
		}
		r.isRan = true
	case "num_el_ran":
		ranRange := strings.Split(ruleString[1], "..")
		if len(ranRange) != 2 {
			return ErrRangeRanMustBeTwo
		}

		var err error
		r.incElSizeRanRangeMin, err = strconv.ParseInt(ranRange[0], 10, 64)
		if err != nil {
			return fmt.Errorf("num el ran min range %w", ErrMustBeNumber)
		}
		r.incElSizeRanRangeMax, err = strconv.ParseInt(ranRange[1], 10, 64)
		if err != nil {
			return fmt.Errorf("num el ran max range %w", ErrMustBeNumber)
		}
		r.isIncElSize = true
	case "num_el_sync":
		r.isSync = true
		r.fieldToSync = ruleString[1]
	case "unixtime_ref":
		var err error
		r.unixTimeRef, err = strconv.ParseInt(ruleString[1], 10, 64)
		if err != nil {
			r.unixTimeRef = time.Now().Unix()
		}
	case "retention":
		var err error
		r.retentionTime, err = strconv.ParseInt(ruleString[1], 10, 64)
		if err != nil {
			r.retentionTime = int64(1)
		}
	}

	return nil
}
