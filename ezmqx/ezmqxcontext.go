/*******************************************************************************
 * Copyright 2018 Samsung Electronics All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 *******************************************************************************/

package ezmqx

import (
	"go.uber.org/zap"
	"go/aml"
	"go/ezmq"

	"container/list"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type EZMQXContext struct {
	initialized atomic.Value
	terminated  atomic.Value
	standAlone  bool
	hostName    string
	hostAddr    string
	remoteAddr  string
	tnsEnabled  bool
	numOfPort   int
	usedIdx     int
	amlRepDic   map[string]*aml.Representation
	usedPorts   map[int]bool
	ports       map[int]int
}

var ctxInstance *EZMQXContext

func getContextInstance() *EZMQXContext {
	if nil == ctxInstance {
		ctxInstance = &EZMQXContext{}
		ctxInstance.initialized.Store(false)
		ctxInstance.standAlone = false
		ctxInstance.amlRepDic = make(map[string]*aml.Representation)
		ctxInstance.usedPorts = make(map[int]bool)
		ctxInstance.ports = make(map[int]int)
	}
	return ctxInstance
}

func (cxtInstance *EZMQXContext) assignDynamicPort() (int, EZMQXErrorCode) {
	port := 0
	for {
		if cxtInstance.numOfPort >= LOCAL_PORT_MAX {
			return -1, EZMQX_MAXIMUM_PORT_EXCEED
		}
		key := LOCAL_PORT_START + cxtInstance.usedIdx
		if true == cxtInstance.usedPorts[key] {
			cxtInstance.usedIdx++
			if cxtInstance.usedIdx >= LOCAL_PORT_MAX {
				cxtInstance.usedIdx = 0
			}
		} else {
			cxtInstance.usedPorts[key] = true
			port = key
			cxtInstance.numOfPort++
			break
		}
	}
	Logger.Debug("Assigned dynamic Port", zap.Int("Port: ", port))
	return port, EZMQX_OK
}

func (contextInstance *EZMQXContext) releaseDynamicPort(port int) EZMQXErrorCode {
	if false == contextInstance.usedPorts[port] {
		return EZMQX_RELEASE_WRONG_PORT
	}
	contextInstance.usedPorts[port] = false
	contextInstance.numOfPort--
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) setHostInfo(name string, address string) {
	ctxInstance.hostAddr = name
	contextInstance.hostName = address
}

func (cxtInstance *EZMQXContext) setTnsInfo(remoteAddr string) {
	cxtInstance.tnsEnabled = true
	cxtInstance.remoteAddr = remoteAddr
}

func (contextInstance *EZMQXContext) parseConfigData(response http.Response) EZMQXErrorCode {
	Logger.Debug("[Config] ", zap.Int(" Status code: ", response.StatusCode))
	if response.StatusCode != HTTP_OK {
		return EZMQX_REST_ERROR
	}
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Logger.Error("[Config] Failed to read response body")
		return EZMQX_REST_ERROR
	}
	Logger.Debug("[Config] ", zap.String("Response: ", string(data)))
	configData := make(map[string][]interface{})
	err = json.Unmarshal([]byte(data), &configData)
	if err != nil {
		Logger.Error("[Config] Json unmarshal failed")
		return EZMQX_REST_ERROR
	}
	config, exists := configData[CONF_PROPS]
	if !exists {
		Logger.Error("[Config] No properties key in json response")
		return EZMQX_REST_ERROR
	}
	anchorKeyExists := false
	nodeKeyExists := false
	for _, item := range config {
		stringMap := item.(map[string]interface{})
		anchorAddress, exists := stringMap[CONF_REMOTE_ADDR]
		if exists {
			Logger.Debug("[Config] ", zap.String("Anchor address: ", anchorAddress.(string)))
			contextInstance.setTnsInfo(anchorAddress.(string))
			anchorKeyExists = true
		}
		nodeAddress, exist := stringMap[CONF_NODE_ADDR]
		if exist {
			contextInstance.hostAddr = nodeAddress.(string)
			Logger.Debug("[Config] ", zap.String("Node address: ", nodeAddress.(string)))
			nodeKeyExists = true
		}
	}
	if !anchorKeyExists || !nodeKeyExists {
		Logger.Error("[Config] Anchor address/ Node address key not exists")
		return EZMQX_REST_ERROR
	}
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) readFromFile(path string) EZMQXErrorCode {
	Logger.Debug("[readFromFile] ", zap.String("File path: ", path))
	data, err := ioutil.ReadFile(path)
	if err != nil {
		Logger.Error("[readFromFile] Unable to read from file")
		return EZMQX_UNKNOWN_STATE
	}
	contextInstance.hostName = string(data)
	//remove trailing /n
	contextInstance.hostName = contextInstance.hostName[0 : len(contextInstance.hostName)-1]
	Logger.Debug("[readFromFile] ", zap.String("hostName: ", contextInstance.hostName))
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) parseAppsResponse(response http.Response) *list.List {
	Logger.Debug("[Running Apps] ", zap.Int(" Status code: ", response.StatusCode))
	if response.StatusCode != HTTP_OK {
		return nil
	}
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Logger.Error("[Running Apps] Failed to read response body")
		return nil
	}
	Logger.Debug("[Running Apps] ", zap.String("Response: ", string(data)))
	result := make(map[string][]interface{})
	err = json.Unmarshal([]byte(data), &result)
	if err != nil {
		Logger.Error("[Running Apps] Json unmarshal failed")
		return nil
	}
	idList := list.New()
	apps, exists := result[APPS_PROPS]
	if !exists {
		Logger.Error("[Running Apps] App properties key not exists")
		return nil
	}
	for _, item := range apps {
		stringMap := item.(map[string]interface{})
		id, exists := stringMap[APPS_ID]
		if !exists {
			Logger.Error("[Running Apps] App ID key not exists")
			return nil
		}
		Logger.Debug("[Running Apps] ", zap.String("id: ", id.(string)))
		state, exists := stringMap[APPS_STATE]
		if !exists {
			Logger.Error("[Running Apps] App State key not exists")
			return nil
		}
		stateString := state.(string)
		Logger.Debug("[Running Apps] ", zap.String("state: ", stateString))
		if 0 == strings.Compare(stateString, APPS_STATE_RUNNING) {
			idList.PushBack(id)
		}
	}
	return idList
}

func (contextInstance *EZMQXContext) parsePortInfo(port interface{}) EZMQXErrorCode {
	ports := port.([]interface{})
	for _, item := range ports {
		stringMap := item.(map[string]interface{})
		privatePort, exists := stringMap[PORTS_PRIVATE]
		if !exists {
			Logger.Error("[Running Apps] No private port key in json response")
			return EZMQX_REST_ERROR
		}
		priPort := strconv.FormatFloat(privatePort.(float64), 'f', -1, 64)
		Logger.Debug("[Port info] ", zap.String("Private port: ", priPort))
		publicPort, exists := stringMap[PORTS_PUBLIC]
		if !exists {
			Logger.Error("[Running Apps] No public port key in json response")
			return EZMQX_REST_ERROR
		}
		pubPort := strconv.FormatFloat(publicPort.(float64), 'f', -1, 64)
		Logger.Debug("[Port info] ", zap.String("Public Port: ", pubPort))
		private, _ := strconv.Atoi(priPort)
		public, _ := strconv.Atoi(pubPort)
		contextInstance.ports[private] = public
	}
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) parseAppInfo(response http.Response) EZMQXErrorCode {
	Logger.Debug("[App info] ", zap.Int(" Status code: ", response.StatusCode))
	if response.StatusCode != HTTP_OK {
		return EZMQX_REST_ERROR
	}
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Logger.Error("[Running Apps] Failed to read response body")
		return EZMQX_REST_ERROR
	}
	Logger.Debug("[App info] ", zap.String("Response: ", string(data)))
	appInfo := make(map[string]interface{})
	err = json.Unmarshal([]byte(data), &appInfo)
	if err != nil {
		Logger.Error("[Running Apps] Unmarshal error")
		return EZMQX_REST_ERROR
	}
	services, exists := appInfo[SERVICES_PROPS]
	if !exists {
		Logger.Error("[Running Apps] No services key in json response")
		return EZMQX_REST_ERROR
	}
	interfaces := services.([]interface{})
	for _, service := range interfaces {
		serviceMap := service.(map[string]interface{})
		cid, exists := serviceMap[SERVICES_CON_ID]
		if !exists {
			Logger.Error("[Running Apps] No id key in json response")
			return EZMQX_REST_ERROR
		}
		hostName := contextInstance.hostName
		containerId := cid.(string)
		containerId = containerId[0:len(hostName)]
		Logger.Debug("[App info] ", zap.String("Container Id: ", containerId))
		Logger.Debug("[App info] ", zap.String("Host name: ", hostName))
		if 0 == strings.Compare(containerId, hostName) {
			port, exists := serviceMap[SERVICES_CON_PORTS]
			if !exists {
				Logger.Error("[Running Apps] No ports key in json response")
				return EZMQX_REST_ERROR
			}
			result := contextInstance.parsePortInfo(port)
			if result != EZMQX_OK {
				Logger.Error("[Running Apps] Parse port info failed")
				return EZMQX_REST_ERROR
			}
		}
	}
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) initializeDockerMode() EZMQXErrorCode {
	ezmqResult := ezmq.GetInstance().Initialize()
	if ezmqResult != ezmq.EZMQ_OK {
		Logger.Error("Could not initialize EZMQ")
		return EZMQX_UNKNOWN_STATE
	}
	timeout := time.Duration(CONNECTION_TIMEOUT * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	// Configuration resource
	configURL := NODE + PREFIX + API_CONFIG
	Logger.Debug("[Config] ", zap.String("Rest URL: ", string(configURL)))
	response, err := client.Get(configURL)
	if err != nil {
		Logger.Error("[Config] HTTP request failed")
		return EZMQX_REST_ERROR
	}
	result := contextInstance.parseConfigData(*response)
	if result != EZMQX_OK {
		Logger.Error("[Config] Parse config data failed ")
		return result
	}
	// Get Host Name
	result = contextInstance.readFromFile(HOST_NAME_FILE_PATH)
	if result != EZMQX_OK {
		Logger.Error("[Config] Read from file failed")
		return result
	}
	// Applications resource
	var idList *list.List = nil
	appsURL := NODE + PREFIX + API_APPS
	Logger.Debug("[Running Apps] ", zap.String("Rest URL: ", string(appsURL)))
	response, err = client.Get(appsURL)
	if err != nil {
		Logger.Error("[Running Apps] HTTP request failed")
		return EZMQX_REST_ERROR
	}
	idList = contextInstance.parseAppsResponse(*response)
	if nil == idList {
		Logger.Error("[Running Apps] Parse apps response failed")
		return EZMQX_REST_ERROR
	}
	// APP info
	appInfoURL := NODE + PREFIX + API_APPS + SLASH
	for id := idList.Front(); id != nil; id = id.Next() {
		appId := id.Value.(string)
		url := appInfoURL + appId
		Logger.Debug("[App Info] ", zap.String("Rest URL: ", url))
		response, err = client.Get(url)
		if err != nil {
			Logger.Error("[App info] HTTP request failed")
			return EZMQX_REST_ERROR
		}
		contextInstance.parseAppInfo(*response)
	}
	contextInstance.initialized.Store(true)
	contextInstance.terminated.Store(false)
	Logger.Debug("EZMQX Context created")
	return EZMQX_OK
}

func (contextInstance *EZMQXContext) initializeStandAloneMode(useTns bool, tnsAddr string) EZMQXErrorCode {
	result := ezmq.GetInstance().Initialize()
	if result != ezmq.EZMQ_OK {
		Logger.Error("Could not start ezmq context")
		return EZMQX_UNKNOWN_STATE
	}
	ctxInstance.standAlone = true
	ctxInstance.setHostInfo(LOCAL_HOST, LOCAL_HOST)
	if useTns {
		ctxInstance.setTnsInfo(tnsAddr)
	}
	ctxInstance.initialized.Store(true)
	ctxInstance.terminated.Store(false)
	Logger.Debug("EZMQX Context created")
	return EZMQX_OK
}

func (cxtInstance *EZMQXContext) getAmlRep(amlModelId string) (*aml.Representation, EZMQXErrorCode) {
	rep := cxtInstance.amlRepDic[amlModelId]
	if nil == rep {
		Logger.Error("No representation found for model ID")
		return nil, EZMQX_UNKNOWN_AML_MODEL
	}
	return rep, EZMQX_OK
}

func (cxtInstance *EZMQXContext) addAmlRep(amlFilePath list.List) (*list.List, EZMQXErrorCode) {
	modelId := list.New()
	for filePath := amlFilePath.Front(); filePath != nil; filePath = filePath.Next() {
		repObject, err := aml.CreateRepresentation(filePath.Value.(string))
		if err != aml.AML_OK {
			Logger.Error("Create representation failed")
			return modelId, EZMQX_INVALID_AML_MODEL
		}
		amlModelId, err := repObject.GetRepresentationId()
		if err != aml.AML_OK {
			Logger.Error("Get representation Id failed")
			return modelId, EZMQX_INVALID_PARAM
		}
		if nil == cxtInstance.amlRepDic[amlModelId] {
			cxtInstance.amlRepDic[amlModelId] = repObject
		}
		modelId.PushBack(amlModelId)
	}
	return modelId, EZMQX_OK
}

func (cxtInstance *EZMQXContext) getHostEp(port int) (*EZMQXEndpoint, EZMQXErrorCode) {
	hostPort := 0
	if cxtInstance.isCtxStandAlone() {
		hostPort = port
	} else {
		if 0 != cxtInstance.ports[port] {
			hostPort = cxtInstance.ports[port]
		}
		if 0 == hostPort {
			return nil, EZMQX_UNKNOWN_STATE
		}
	}
	endPoint := GetEZMQXEndPoint1(cxtInstance.hostAddr, hostPort)
	return endPoint, EZMQX_OK
}

func (cxtInstance *EZMQXContext) terminate() EZMQXErrorCode {
	if true == cxtInstance.terminated.Load() {
		Logger.Debug("Context already terminated")
		return EZMQX_TERMINATED
	}

	//terminate topic handler
	topicHandler := getTopicHandler()
	topicHandler.terminateHandler()
	Logger.Debug("Terminated handler")

	//clear maps
	for key := range cxtInstance.ports {
		delete(cxtInstance.ports, key)
	}
	for key := range cxtInstance.usedPorts {
		delete(cxtInstance.usedPorts, key)
	}
	for key := range cxtInstance.amlRepDic {
		delete(cxtInstance.amlRepDic, key)
	}
	cxtInstance.hostName = ""
	cxtInstance.hostAddr = ""
	cxtInstance.remoteAddr = ""
	cxtInstance.usedIdx = 0
	cxtInstance.numOfPort = 0
	cxtInstance.standAlone = false
	cxtInstance.tnsEnabled = false
	Logger.Debug("Try EZMQ API terminate")
	if ezmq.EZMQ_OK != ezmq.GetInstance().Terminate() {
		Logger.Debug("EZMQ API terminate failed")
	}
	Logger.Debug("EZMQ API terminated")
	cxtInstance.terminated.Store(true)
	cxtInstance.initialized.Store(false)
	Logger.Debug("EZMQX Context terminated")
	return EZMQX_OK
}

func (cxtInstance *EZMQXContext) isCtxInitialized() bool {
	return (cxtInstance.initialized.Load()).(bool)
}

func (cxtInstance *EZMQXContext) isCtxTerminated() bool {
	return (cxtInstance.terminated.Load()).(bool)
}

func (cxtInstance *EZMQXContext) isCtxStandAlone() bool {
	return cxtInstance.standAlone
}

func (cxtInstance *EZMQXContext) isCtxTnsEnabled() bool {
	return cxtInstance.tnsEnabled
}

func (cxtInstance *EZMQXContext) ctxGetTnsAddr() string {
	return cxtInstance.remoteAddr
}
