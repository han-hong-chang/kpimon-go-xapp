package control

/*
#include <e2sm/wrapper.h>
#cgo LDFLAGS: -le2smwrapper -lm
#cgo CFLAGS: -I/usr/local/include/e2sm
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"gerrit.o-ran-sc.org/r/ric-plt/xapp-frame/pkg/clientmodel"
	"gerrit.o-ran-sc.org/r/ric-plt/xapp-frame/pkg/xapp"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

type Control struct {
	RMR    chan *xapp.RMRParams //channel for receiving rmr message
	client influxdb2.Client     //client for influxdb
}

var (
	influxDBAddress      = "http://r4-influxdb-influxdb2.ricplt:80"
	influxDBUser         = "admin"
	influxDBPassword     = "admin"
	influxDBOrganization = "my-org"
	influxDBBucket       = "kpimon"
	influxDBToken        = ""
	logLevel             = int(4)
	actionType           = "report"
	actionId             = int64(1)
	seqId                = int64(1)
	funcId               = int64(2)
	Glob_cell            = make(map[string]bool)
	hPort                = int64(8080)
	rPort                = int64(4560)
	clientEndpoint       = clientmodel.SubscriptionParamsClientEndpoint{
		Host:     "service-ricxapp-kpimon-go-http.ricxapp",
		HTTPPort: &hPort,
		RMRPort:  &rPort,
	}
)

func NewControl() Control {
	xapp.Logger.Info("In new control\n")
	//Get configuration
	logLevel = xapp.Config.GetInt("logger.level")
	influxDBAddress = xapp.Config.GetString("influxDB.influxDBAddress")
	influxDBUser = xapp.Config.GetString("influxDB.username")
	influxDBPassword = xapp.Config.GetString("influxDB.password")
	influxDBOrganization = xapp.Config.GetString("influxDB.organization")
	influxDBBucket = xapp.Config.GetString("influxDB.bucket")
	influxDBToken = xapp.Config.GetString("influxDB.token")

	//Set log level
	xapp.Logger.SetLevel(logLevel)

	//Initial control structure
	return Control{
		make(chan *xapp.RMRParams),
		influxdb2.NewClient(influxDBAddress, influxDBToken), //fmt.Sprintf("%s:%s", influxDBUser, influxDBPassword)),
	}
}

func (c Control) getEnbList() ([]*xapp.RNIBNbIdentity, error) {
	enbs, err := xapp.Rnib.GetListEnbIds()
	if err != nil {
		xapp.Logger.Error("err: %s", err)
		return nil, err
	}

	xapp.Logger.Info("List for connected eNBs :")
	for index, enb := range enbs {
		xapp.Logger.Info("%d. enbid: %s", index+1, enb.InventoryName)
	}
	return enbs, nil
}

func (c *Control) getGnbList() ([]*xapp.RNIBNbIdentity, error) {
	gnbs, err := xapp.Rnib.GetListGnbIds()

	if err != nil {
		xapp.Logger.Error("err: %s", err)
		return nil, err
	}
	xapp.Logger.Info("List of connected gNBs :")
	for index, gnb := range gnbs {
		xapp.Logger.Info("%d. gnbid : %s", index+1, gnb.InventoryName)
	}
	return gnbs, nil
}

func (c *Control) getnbList() []*xapp.RNIBNbIdentity {
	//Get all GnodeB and EnodeB connected to RIC
	var nbs []*xapp.RNIBNbIdentity

	if enbs, err := c.getEnbList(); err == nil {
		nbs = append(nbs, enbs...)
	}

	if gnbs, err := c.getGnbList(); err == nil {
		nbs = append(nbs, gnbs...)
	}
	return nbs
}

func cellid_to_list_of_int(str string) []int64 {
	l := len(str)
	var ans []int64
	for i := 0; i < l; i += 2 {
		output, err := strconv.ParseInt(str[i:i+2], 16, 64)
		if err != nil {
			fmt.Println(err)
			return ans
		}
		ans = append(ans, output)
	}
	return ans
}

func plmnid_to_list_of_int(str string) []int64 {
	l := len(str)
	var ans []int64
	for i := 0; i < l; i += 2 {
		output, err := strconv.ParseInt(str[i:i+2], 16, 64)
		if err != nil {
			fmt.Println(err)
			return ans
		}
		ans = append(ans, output)
	}
	return ans
}

func encode_action_format1(plmn string, cellid string) clientmodel.ActionDefinition {

	lol1 := plmnid_to_list_of_int(plmn)
	lol2 := cellid_to_list_of_int(cellid)
	var format1 []int64

	//for simulation-by measName(supported in Viavi 1.6.1)
	//format1 = []int64{0, 1, 1, 8, 0, 22, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 1, 32, 0, 0, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 1, 32, 0, 0, 0, 176, 80, 69, 69, 46, 65, 118, 103, 80, 111, 119, 101, 114, 1, 32, 0, 0, 0, 144, 80, 69, 69, 46, 69, 110, 101, 114, 103, 121, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 85, 108, 1, 32, 0, 0, 0, 160, 82, 82, 67, 46, 67, 111, 110, 110, 77, 97, 120, 1, 32, 0, 0, 0, 176, 82, 82, 67, 46, 67, 111, 110, 110, 77, 101, 97, 110, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 68, 108, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 85, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 68, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 85, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 1, 32, 0, 0, 0, 192, 86, 105, 97, 118, 105, 46, 71, 110, 98, 68, 117, 73, 100, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 67, 103, 105, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 80, 99, 105, 1, 32, 0, 0, 1, 96, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 110, 116, 101, 110, 110, 97, 84, 121, 112, 101, 1, 32, 0, 0, 1, 32, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 122, 105, 109, 117, 116, 104, 1, 32, 0, 0, 1, 0, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 112, 111, 119, 101, 114, 1, 32, 0, 0, 64, 39, 15, 0} //assuming nr cells

	//granuPeriod = 10000ms
	//format1 = []int64{0, 1, 1, 8, 0, 24, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 1, 32, 0, 0, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 1, 32, 0, 0, 0, 176, 80, 69, 69, 46, 65, 118, 103, 80, 111, 119, 101, 114, 1, 32, 0, 0, 0, 144, 80, 69, 69, 46, 69, 110, 101, 114, 103, 121, 1, 32, 0, 0, 1, 144, 86, 105, 97, 118, 105, 46, 80, 69, 69, 46, 69, 110, 101, 114, 103, 121, 69, 102, 102, 105, 99, 105, 101, 110, 99, 121, 1, 32, 0, 0, 0, 224, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 99, 111, 114, 101, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 85, 108, 1, 32, 0, 0, 0, 160, 82, 82, 67, 46, 67, 111, 110, 110, 77, 97, 120, 1, 32, 0, 0, 0, 176, 82, 82, 67, 46, 67, 111, 110, 110, 77, 101, 97, 110, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 68, 108, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 85, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 68, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 85, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 1, 32, 0, 0, 0, 192, 86, 105, 97, 118, 105, 46, 71, 110, 98, 68, 117, 73, 100, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 67, 103, 105, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 80, 99, 105, 1, 32, 0, 0, 1, 96, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 110, 116, 101, 110, 110, 97, 84, 121, 112, 101, 1, 32, 0, 0, 1, 32, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 122, 105, 109, 117, 116, 104, 1, 32, 0, 0, 1, 0, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 112, 111, 119, 101, 114, 1, 32, 0, 0, 64, 39, 15, 0}

	//For All
	//granuPeriod = 1000ms
	//format1 = []int64{0, 1, 1, 8, 0, 24, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 1, 32, 0, 0, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 1, 32, 0, 0, 0, 176, 80, 69, 69, 46, 65, 118, 103, 80, 111, 119, 101, 114, 1, 32, 0, 0, 0, 144, 80, 69, 69, 46, 69, 110, 101, 114, 103, 121, 1, 32, 0, 0, 1, 144, 86, 105, 97, 118, 105, 46, 80, 69, 69, 46, 69, 110, 101, 114, 103, 121, 69, 102, 102, 105, 99, 105, 101, 110, 99, 121, 1, 32, 0, 0, 0, 224, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 99, 111, 114, 101, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 1, 32, 0, 0, 1, 144, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 85, 108, 1, 32, 0, 0, 0, 160, 82, 82, 67, 46, 67, 111, 110, 110, 77, 97, 120, 1, 32, 0, 0, 0, 176, 82, 82, 67, 46, 67, 111, 110, 110, 77, 101, 97, 110, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 68, 108, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 85, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 68, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 85, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 1, 32, 0, 0, 0, 192, 86, 105, 97, 118, 105, 46, 71, 110, 98, 68, 117, 73, 100, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 67, 103, 105, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 80, 99, 105, 1, 32, 0, 0, 1, 96, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 110, 116, 101, 110, 110, 97, 84, 121, 112, 101, 1, 32, 0, 0, 1, 32, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 97, 122, 105, 109, 117, 116, 104, 1, 32, 0, 0, 1, 0, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 112, 111, 119, 101, 114, 1, 32, 0, 0, 64, 3, 231, 0}

	//For SLA
	//granuPeriod = 1000ms
	format1 = []int64{0, 1, 1, 8, 0, 10, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 1, 32, 0, 0, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 68, 108, 1, 32, 0, 0, 0, 208, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 85, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 68, 108, 1, 32, 0, 0, 0, 176, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 85, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 1, 32, 0, 0, 0, 192, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 1, 32, 0, 0, 0, 192, 86, 105, 97, 118, 105, 46, 71, 110, 98, 68, 117, 73, 100, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 67, 103, 105, 1, 32, 0, 0, 0, 160, 86, 105, 97, 118, 105, 46, 78, 114, 80, 99, 105, 1, 32, 0, 0, 64, 3, 231, 0}

	//appending plmn
	format1 = append(format1, lol1...)

	//appending cellid
	format1 = append(format1, lol2...)

	return format1
}

func encode_action_format2() clientmodel.ActionDefinition {
	var format2 []int64
	format2 = []int64{0, 1, 0, 0, 0, 20, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 1, 0, 0, 0, 1, 64, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 1, 0, 0, 0, 1, 0, 71, 78, 66, 45, 68, 85, 45, 73, 68, 1, 0, 0, 0, 0, 160, 78, 82, 45, 67, 71, 73, 1, 0, 0, 0, 0, 160, 78, 82, 45, 80, 67, 73, 1, 0, 0, 0, 2, 192, 81, 111, 115, 70, 108, 111, 119, 46, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 1, 0, 0, 0, 2, 192, 81, 111, 115, 70, 108, 111, 119, 46, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 85, 108, 1, 0, 0, 0, 1, 64, 82, 82, 67, 46, 67, 111, 110, 110, 77, 97, 120, 1, 0, 0, 0, 1, 96, 82, 82, 67, 46, 67, 111, 110, 110, 77, 101, 97, 110, 1, 0, 0, 0, 1, 160, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 68, 108, 1, 0, 0, 0, 1, 160, 82, 82, 85, 46, 80, 114, 98, 65, 118, 97, 105, 108, 85, 108, 1, 0, 0, 0, 1, 32, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 1, 0, 0, 0, 1, 96, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 68, 108, 1, 0, 0, 0, 1, 96, 82, 82, 85, 46, 80, 114, 98, 84, 111, 116, 85, 108, 1, 0, 0, 0, 1, 128, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 1, 0, 0, 0, 1, 128, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 1, 0, 0, 0, 1, 64, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 1, 0, 0, 0, 1, 64, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 1, 0, 0, 0, 1, 64, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 1, 0, 0, 0, 2, 0, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 112, 111, 119, 101, 114, 1, 0, 0, 0, 2, 64, 86, 105, 97, 118, 105, 46, 82, 97, 100, 105, 111, 46, 115, 101, 99, 116, 111, 114, 115, 1, 0, 0, 0, 0, 0}
	//encode the variable part and append it to our array.
	format2 = append(format2, 89) //appending variable part if necessory
	return format2
}

func encode_action_format3() clientmodel.ActionDefinition {
	var format3 []int64

	// for simulation-by measName(supported in VIAVI RIC Test 1.6.1), Currently, VIAVI only support noLabel and all UE without specific cell.
	// RIC Test v1.6.1 bug, skip the measure name of "Viavi_UE_servingDistance" and "Viavi_UE_speed"
	//granuPeriod = 10000ms
	//format3 = []int64{0, 1, 3, 64, 0, 39, 0, 160, 68, 82, 66, 46, 85, 69, 67, 113, 105, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 67, 113, 105, 85, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 200, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 80, 84, 66, 46, 84, 111, 116, 78, 98, 114, 68, 108, 0, 0, 16, 0, 0, 0, 80, 84, 66, 46, 84, 111, 116, 78, 98, 114, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 53, 113, 105, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 67, 101, 108, 108, 73, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 68, 114, 98, 73, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 71, 102, 98, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 66, 101, 97, 109, 73, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 208, 86, 105, 97, 118, 105, 46, 85, 69, 46, 116, 97, 114, 103, 101, 116, 84, 104, 114, 111, 117, 103, 104, 112, 117, 116, 68, 108, 0, 0, 16, 0, 0, 0, 208, 86, 105, 97, 118, 105, 46, 85, 69, 46, 116, 97, 114, 103, 101, 116, 84, 104, 114, 111, 117, 103, 104, 112, 117, 116, 85, 108, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 105, 100, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 99, 111, 114, 101, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 77, 102, 98, 114, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 80, 114, 105, 111, 114, 105, 116, 121, 0, 0, 16, 0, 0, 0, 128, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 108, 105, 99, 101, 73, 100, 0, 0, 16, 0, 0, 0, 152, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 84, 97, 114, 103, 101, 116, 84, 112, 117, 116, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 85, 101, 82, 110, 116, 105, 0, 0, 16, 0, 0, 32, 39, 15}

	//For All
	//granuPeriod = 1000ms
	//format3 = []int64{0, 1, 3, 64, 0, 39, 0, 160, 68, 82, 66, 46, 85, 69, 67, 113, 105, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 67, 113, 105, 85, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 200, 81, 111, 115, 70, 108, 111, 119, 46, 84, 111, 116, 80, 100, 99, 112, 80, 100, 117, 86, 111, 108, 117, 109, 101, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 80, 84, 66, 46, 84, 111, 116, 78, 98, 114, 68, 108, 0, 0, 16, 0, 0, 0, 80, 84, 66, 46, 84, 111, 116, 78, 98, 114, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 120, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 121, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 71, 101, 111, 46, 122, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 53, 113, 105, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 67, 101, 108, 108, 73, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 68, 114, 98, 73, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 71, 102, 98, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 66, 101, 97, 109, 73, 100, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 208, 86, 105, 97, 118, 105, 46, 85, 69, 46, 116, 97, 114, 103, 101, 116, 84, 104, 114, 111, 117, 103, 104, 112, 117, 116, 68, 108, 0, 0, 16, 0, 0, 0, 208, 86, 105, 97, 118, 105, 46, 85, 69, 46, 116, 97, 114, 103, 101, 116, 84, 104, 114, 111, 117, 103, 104, 112, 117, 116, 85, 108, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 105, 100, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 99, 111, 114, 101, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 77, 102, 98, 114, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 80, 114, 105, 111, 114, 105, 116, 121, 0, 0, 16, 0, 0, 0, 128, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 83, 108, 105, 99, 101, 73, 100, 0, 0, 16, 0, 0, 0, 152, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 84, 97, 114, 103, 101, 116, 84, 112, 117, 116, 0, 0, 16, 0, 0, 0, 120, 86, 105, 97, 118, 105, 46, 81, 111, 83, 46, 85, 101, 82, 110, 116, 105, 0, 0, 16, 0, 0, 32, 3, 231}

	//For SLA
	//granuPeriod = 1000ms
	//format3 = []int64{0, 1, 3, 64, 0, 6, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test add anomaliy (o)
	//format3 = []int64{0, 1, 3, 64, 0, 7, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test add anomaliy, rsrp (o)
	//format3 = []int64{0, 1, 3, 64, 0, 8, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test anomaliy, rsrp, rsrq, RsSinr
	//format3 = []int64{0, 1, 3, 64, 0, 10, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test anomalit, rsrp, rsrq, RsSinr, Nb0
	//format3 = []int64{0, 1, 3, 64, 0, 11, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test anomalit, rsrp, rsrq, RsSinr, Nb0, Nb1
	//format3 = []int64{0, 1, 3, 64, 0, 12, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 105, 100, 0, 0, 16, 0, 0, 32, 3, 231}

	//For test anomly, rsrp, rsrq, RsSinr, NB0, NB1, NB0_rsrp, NB1_rsrp
	//format3 = []int64{0, 1, 3, 64, 0, 14, 0, 160, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 83, 108, 105, 99, 101, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 105, 100, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 32, 3, 231}

	//Test order
	format3 = []int64{0, 1, 3, 64, 0, 13, 0, 160, 86, 105, 97, 118, 105, 46, 85, 69, 46, 105, 100, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 67, 101, 108, 108, 46, 105, 100, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 105, 100, 0, 0, 16, 0, 0, 0, 88, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 105, 100, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 68, 108, 0, 0, 16, 0, 0, 0, 80, 68, 82, 66, 46, 85, 69, 84, 104, 112, 85, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 68, 108, 0, 0, 16, 0, 0, 0, 96, 82, 82, 85, 46, 80, 114, 98, 85, 115, 101, 100, 85, 108, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 96, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 114, 113, 0, 0, 16, 0, 0, 0, 112, 86, 105, 97, 118, 105, 46, 85, 69, 46, 82, 115, 83, 105, 110, 114, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 49, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 104, 86, 105, 97, 118, 105, 46, 78, 98, 50, 46, 82, 115, 114, 112, 0, 0, 16, 0, 0, 0, 136, 86, 105, 97, 118, 105, 46, 85, 69, 46, 97, 110, 111, 109, 97, 108, 105, 101, 115, 0, 0, 16, 0, 0, 32, 3, 231}
	return format3
}

func query_all_cell_id(meid string) (plmd string, cells []string) {
	// change to xapp.api
	link := "http://service-ricplt-e2mgr-http.ricplt.svc.cluster.local:3800/v1/nodeb/"
	link = link + meid
	tmpr, err := http.Get(link)
	if err != nil {
		log.Fatalln(err)
		return "", make([]string, 0)
	}
	defer tmpr.Body.Close()
	var resp E2mgrResponse

	err = json.NewDecoder(tmpr.Body).Decode(&resp)
	if err != nil {
		log.Fatalln(err)
		return "", make([]string, 0)
	}

	counter := 0
	for i := 0; i < len(resp.Gnb.NodeConfigs); i++ {
		// change "f1" to "xn" to get all the cell Id
		if resp.Gnb.NodeConfigs[i].E2nodeComponentInterfaceType == "xn" {
			counter = i
			break
		}
	}
	tm := resp.Gnb.NodeConfigs[counter].E2nodeComponentRequestPart
	base64Text := make([]byte, base64.StdEncoding.DecodedLen(len(tm)))
	nl, _ := base64.StdEncoding.Decode(base64Text, []byte(tm))
	message := string(base64Text[:nl])

	counter = 0
	for i := 0; i < len(meid); i++ {
		if meid[i] == '_' {
			counter++
		}
		if counter == 3 {
			counter = i + 1
			break
		}
	}

	ans := strings.ToUpper(meid[counter:len(meid)])
	l1 := int64(len(message))
	l2 := int64(len(ans))

	for i := int64(0); i <= l1-l2; i++ {
		if strings.Contains(message[i:i+l2], ans) {
			Glob_cell[message[i:i+10]] = true
			cells = append(cells, message[i:i+10])
			fmt.Println(message[i : i+10])
		}
	}
	return resp.GlobalNbId.PlmnId, cells
}

func (c Control) handleSubscription(meid string, actions clientmodel.ActionsToBeSetup) {
	subscritionParams := clientmodel.SubscriptionParams{
		ClientEndpoint: &clientEndpoint,
		Meid:           &meid,
		RANFunctionID:  &funcId,
		SubscriptionDetails: clientmodel.SubscriptionDetailsList{
			&clientmodel.SubscriptionDetail{
				EventTriggers: clientmodel.EventTriggerDefinition{
					0, 99, //8,39,15, for 10000 ms reporting period, 0, 99 for 1000ms reporting period
				},
				XappEventInstanceID: &seqId,
				ActionToBeSetupList: actions,
			},
		},
	}

	b, err := json.MarshalIndent(subscritionParams, "", " ")
	if err != nil {
		xapp.Logger.Error("Json marshaling failed: %v", err)
	}
	xapp.Logger.Info("*****body: %s", string(b))

	resp, err := xapp.Subscription.Subscribe(&subscritionParams)
	if err != nil {
		xapp.Logger.Error("Subscription (%s) failed  with error: %s", meid, err)
		return
	}
	xapp.Logger.Info("Successfully subscription done (%s), subscriptrion id: %s", meid, *resp.SubscriptionID)
}

func (c Control) sendSubscription(meid string) {
	//Create Subscription message and send it to RIC platform
	xapp.Logger.Info("Sending subscription request for MEID: %v", meid)

	// Due to submgr only allow at most 16 action definitions for one subscription request.
	// We can generate the actiondefinition first, then divide into multi-subscription by 10.

	actions := clientmodel.ActionsToBeSetup{}

	// Prepare for the action format 1
	// query all cell id
	plmn, cells := query_all_cell_id(meid)
	actioncount := int64(0)

	for i := 0; i < len(cells); i++ {
		actioncount++
		tmp := actioncount
		actionId := &tmp

		action := clientmodel.ActionToBeSetup{
			ActionID:         actionId,
			ActionType:       &actionType,
			ActionDefinition: encode_action_format1(plmn, cells[i]),
			SubsequentAction: nil,
		}

		actions = append(actions, &action)
	}

	// Prepare for the action format 3
	actioncount++
	tmp := actioncount
	actionId := &tmp

	action := clientmodel.ActionToBeSetup{
		ActionID:         actionId,
		ActionType:       &actionType,
		ActionDefinition: encode_action_format3(),
		SubsequentAction: nil,
	}

	actions = append(actions, &action)

	// Divide into sub group
	for i := 0; i < len(actions); i += 10 {
		subActions := clientmodel.ActionsToBeSetup{}
		for j := 0; j < 10 && i+j < len(actions); j++ {
			subActions = append(subActions, actions[i+j])
		}
		c.handleSubscription(meid, subActions)
	}

}

func (c *Control) controlLoop() {
	//Handle receiving message based on message type
	for {
		msg := <-c.RMR
		xapp.Logger.Debug("Received message type: %d", msg.Mtype)
		switch msg.Mtype {
		case xapp.RIC_INDICATION:
			go c.handleIndication(msg)
		default:
			xapp.Logger.Error("Unknown Message Type '%d', discarding", msg.Mtype)
		}
	}
}

func (c Control) Consume(msg *xapp.RMRParams) error {
	id := xapp.Rmr.GetRicMessageName(msg.Mtype)
	xapp.Logger.Info(
		"Message received: name=%s meid=%s subId=%d txid=%s len=%d",
		id,
		msg.Meid.RanName,
		msg.SubId,
		msg.Xid,
		msg.PayloadLen,
	)
	c.RMR <- msg
	return nil
}

func (c *Control) handleIndication(params *xapp.RMRParams) (err error) {
	var e2ap *E2ap
	//var e2sm *E2sm

	indicationMsg, err := e2ap.GetIndicationMessage(params.Payload)
	if err != nil {
		xapp.Logger.Error("Failed to decode RIC Indication message: %v", err)
		return
	}

	log.Printf("RIC Indication message from {%s} received", params.Meid.RanName)
	/*
		indicationHdr, err := e2sm.GetIndicationHeader(indicationMsg.IndHeader)
		if err != nil {
			xapp.Logger.Error("Failed to decode RIC Indication Header: %v", err)
			return
		}
	*/

	//Decoding message and put information into log
	//log.Printf("-----------RIC Indication Header-----------")
	//log.Printf("indicationMsg.IndHeader= %x", indicationMsg.IndHeader)
	/*
	   buf := new(bytes.Buffer) //create my buffer
	   binary.Write(buf, binary.LittleEndian, indicationMsg.IndHeader)
	   log.Printf("binary Write buf= %x",buf )
	   b := buf.Bytes()
	   //str := buf.String()
	   //log.Printf(" buf Strin()= %s",str )
	   //cptr1:= unsafe.Pointer(C.CString(str))
	   cptr1:= unsafe.Pointer(&b[0])
	   defer C.free(cptr1)
	*/
	var timestamp int64
	cptr1 := unsafe.Pointer(&indicationMsg.IndHeader[0])
	decodedHdr := C.e2sm_decode_ric_indication_header(cptr1, C.size_t(len(indicationMsg.IndHeader)))
	//decodedHdr := C.e2sm_decode_ric_indication_header(cptr1, C.size_t(len(str)))
	//decodedHdr := C.e2sm_decode_ric_indication_header(cptr1, C.size_t(buf.Len()))
	if decodedHdr == nil {
		return errors.New("e2sm wrapper is unable to get IndicationHeader due to wrong or invalid input")
	}
	defer C.e2sm_free_ric_indication_header(decodedHdr)
	IndHdrType := int32(decodedHdr.indicationHeader_formats.present)
	if IndHdrType == 0 {
		log.Printf("No Indication Header present")
	}
	if IndHdrType == 1 {
		log.Printf("Indication Header format = %d", IndHdrType)

		indHdrFormat1_C := *(**C.E2SM_KPM_IndicationHeader_Format1_t)(unsafe.Pointer(&decodedHdr.indicationHeader_formats.choice[0]))
		// Handle colletStartTime
		colletStartTime := C.GoBytes(unsafe.Pointer(indHdrFormat1_C.colletStartTime.buf), C.int(indHdrFormat1_C.colletStartTime.size))
		ts := append([]byte{0, 0, 0, 0}, colletStartTime...)
		var value int64
		err = binary.Read(bytes.NewReader(ts), binary.BigEndian, &value)
		if err != nil {
			log.Println("Failed to parse the timestamp: %v", err)
			return err
		}

		//Due to RIC Test error, need to minus a threshold.
		value -= 2208988800
		//Convert UTC 32bits to UTC 64bits
		value *= int64(time.Second)
		log.Printf("Successfully parse the timestamp = %v => %v", colletStartTime, value)

		timestamp = value
		/*
		   indHdrFormat1_C := *(**C.E2SM_KPM_IndicationHeader_Format1_t)(unsafe.Pointer(&decodedHdr.indicationHeader_formats.choice[0]))
		   //senderName_C := (*C.PrintableString_t)(unsafe.Pointer(indHdrFormat1_C.senderName))
		   senderName_C:=indHdrFormat1_C.senderName
		   var senderName []byte
		   senderName = C.GoBytes(unsafe.Pointer(senderName_C.buf), C.int(senderName_C.size))
		   //log.Printf("Sender Name = %x",senderName)

		   //senderType_C := (*C.PrintableString_t)(unsafe.Pointer(indHdrFormat1_C.senderType))
		   senderType_C :=indHdrFormat1_C.senderType
		   //senderType []byte
		   senderType := C.GoBytes(unsafe.Pointer(senderType_C.buf), C.int(senderType_C.size))
		   //log.Printf("Sender Type = %x",senderType)

		   //vendorName_C := (*C.PrintableString_t)(unsafe.Pointer(indHdrFormat1_C.vendorName))
		   vendorName_C :=indHdrFormat1_C.vendorName
		   //vendorName  []byte
		   vendorName := C.GoBytes(unsafe.Pointer(vendorName_C.buf), C.int(vendorName_C.size))
		   //log.Printf("Vendor Name = %x",vendorName)
		*/

	}

	/*
	   indMsg, err := e2sm.GetIndicationMessage(indicationMsg.IndMessage)
	   if err != nil {
	           xapp.Logger.Error("Failed to decode RIC Indication Message: %v", err)
	           return
	   }
	*/
	//log.Printf("-----------RIC Indication Message-----------")
	//log.Printf("indicationMsg.IndMessage= %x",indicationMsg.IndMessage)
	cptr2 := unsafe.Pointer(&indicationMsg.IndMessage[0])
	indicationmessage := C.e2sm_decode_ric_indication_message(cptr2, C.size_t(len(indicationMsg.IndMessage)))
	if indicationmessage == nil {
		return errors.New("e2sm wrapper is unable to get IndicationMessage due to wrong or invalid input")
	}
	defer C.e2sm_free_ric_indication_message(indicationmessage)
	IndMsgType := int32(indicationmessage.indicationMessage_formats.present)
	if IndMsgType == 1 { //parsing cell metrics
		fmt.Printf(" parsing for cell metrics\n")
		indMsgFormat1_C := *(**C.E2SM_KPM_IndicationMessage_Format1_t)(unsafe.Pointer(&indicationmessage.indicationMessage_formats.choice[0]))
		no_of_cell := int32(indMsgFormat1_C.measData.list.count)
		fmt.Printf(" \n No of cell = %d\n", no_of_cell)
		//fmt.Println(no_of_cell)
		for n := int32(0); n < no_of_cell; n++ {
			var sizeof_MeasurementDataItem_t *C.MeasurementDataItem_t
			MeasurementDataItem_C := *(**C.MeasurementDataItem_t)(unsafe.Pointer(uintptr(unsafe.Pointer(indMsgFormat1_C.measData.list.array)) + (uintptr)(int(n))*unsafe.Sizeof(sizeof_MeasurementDataItem_t)))
			no_of_cell_metrics := int32(MeasurementDataItem_C.measRecord.list.count)
			var CellM CellMetricsEntry
			v := reflect.ValueOf(CellM)
			fmt.Printf(" \n No of cell metrics = %d\n", no_of_cell_metrics)
			values := make(map[string]interface{}, v.NumField())
			//assert no_of_cell_metrics == v.NumField()   they both should be equal.
			if int(no_of_cell_metrics) != v.NumField() {
				log.Printf("no_of_cell_metrics != v.NumField()")
				return errors.New("no_of_cell_metrics != v.NumField()")
			}
			for i := int32(0); i < no_of_cell_metrics; i++ {
				//fmt.Println(i)
				if v.Field(int(i)).CanInterface() {
					var sizeof_MeasurementRecordItem_t *C.MeasurementRecordItem_t
					MeasurementRecordItem_C := *(**C.MeasurementRecordItem_t)(unsafe.Pointer(uintptr(unsafe.Pointer(MeasurementDataItem_C.measRecord.list.array)) + (uintptr)(int(i))*unsafe.Sizeof(sizeof_MeasurementRecordItem_t)))
					type_var := int(MeasurementRecordItem_C.present)
					if type_var == 1 {
						var cast_integer *C.long = (*C.long)(unsafe.Pointer(&MeasurementRecordItem_C.choice[0]))
						values[v.Type().Field(int(i)).Name] = int32(*cast_integer)
					} else if type_var == 2 {
						var cast_float *C.double = (*C.double)(unsafe.Pointer(&MeasurementRecordItem_C.choice[0]))
						values[v.Type().Field(int(i)).Name] = float64(*cast_float)
					} else {
						fmt.Printf("Wrong Data Type")
					}

				} else {
					fmt.Printf("sorry you have a unexported field (lower case) value you are trying to sneak past. Can not allow it: %v\n", v.Type().Field(int(i)).Name)
				}
			} //end of inner for loop

			fmt.Println(values)
			fmt.Printf("Parsing Cell Metric Done")
			c.writeCellMetrics_db(&values, timestamp) //push cellmetrics map entry to database.
		} //end of outer for loop
		//end of if IndMsgType==1 , parsing cell metrics done

	} else if IndMsgType == 2 { //parsing ue metrics

		fmt.Printf(" parsing for UE metrics")

		indMsgFormat2_C := *(**C.E2SM_KPM_IndicationMessage_Format2_t)(unsafe.Pointer(&indicationmessage.indicationMessage_formats.choice[0]))
		MeasurementDataItem_C := *(**C.MeasurementDataItem_t)(unsafe.Pointer(uintptr(unsafe.Pointer(indMsgFormat2_C.measData.list.array))))

		no_of_measurement := int32(MeasurementDataItem_C.measRecord.list.count)
		no_of_ue_metrics := int32(indMsgFormat2_C.measCondUEidList.list.count)
		no_of_ue := no_of_measurement / no_of_ue_metrics
		fmt.Printf(" \n Number of UE  = %d, Numer of Measurement = %d, Number of UE Metrics = %d\n", no_of_ue, no_of_measurement, no_of_ue_metrics)

		for n := int32(0); n < no_of_ue; n++ {
			var UeM UeMetricsEntry
			v := reflect.ValueOf(UeM)
			values := make(map[string]interface{}, v.NumField())
			//assert no_of_ue_metrics == v.NumField()   they both should be equal.
			if int(no_of_ue_metrics) != v.NumField() {
				log.Printf("no_of_ue_metrics != v.NumField()")
				return errors.New("no_of_ue_metrics != v.NumField()")
			}
			for i := int32(0); i < no_of_ue_metrics; i++ {
				//fmt.Println(i)
				if v.Field(int(i)).CanInterface() {
					var sizeof_MeasurementRecordItem_t *C.MeasurementRecordItem_t
					MeasurementRecordItem_C := *(**C.MeasurementRecordItem_t)(unsafe.Pointer(uintptr(unsafe.Pointer(MeasurementDataItem_C.measRecord.list.array)) + (uintptr)(n+no_of_ue*i)*unsafe.Sizeof(sizeof_MeasurementRecordItem_t)))

					type_var := int(MeasurementRecordItem_C.present)
					if type_var == 1 {
						var cast_integer *C.long = (*C.long)(unsafe.Pointer(&MeasurementRecordItem_C.choice[0]))
						values[v.Type().Field(int(i)).Name] = int32(*cast_integer)
					} else if type_var == 2 {
						var cast_float *C.double = (*C.double)(unsafe.Pointer(&MeasurementRecordItem_C.choice[0]))
						values[v.Type().Field(int(i)).Name] = float64(*cast_float)

					} else {
						fmt.Printf("Wrong Data Type")
					}

				} else {
					fmt.Printf("sorry you have a unexported field (lower case) value you are trying to sneak past. Can not allow it: %v\n", v.Type().Field(int(i)).Name)
				}

			} //end of inner for loop
			fmt.Println(values)
			fmt.Printf("Parsing UE Metric Done")
			c.writeUeMetrics_db(&values, timestamp) //push UEmetrics map entry to database.

		} // end of outer for loop
		//parsing ue metrics done
	} else {
		fmt.Printf(" Invalid Indication message format")

	}

	return nil
}

func (c *Control) writeUeMetrics_db(ueMetricsMap *map[string]interface{}, timestamp int64) {
	writeAPI := c.client.WriteAPIBlocking(influxDBOrganization, influxDBBucket)

	// Convert map to JSON
	ueMetricsJSON, err := json.Marshal(ueMetricsMap)
	if err != nil {
		xapp.Logger.Error("Marshal UE Metrics failed!")
	}

	// Convert JSON to Structure 'UeMetricsEntry'
	var ueMetric UeMetricsEntry
	err = json.Unmarshal(ueMetricsJSON, &ueMetric)
	if err != nil {
		xapp.Logger.Error("Unmarshal UE Metrics failed!")
	}

	p := influxdb2.NewPointWithMeasurement("UeMetrics").
		/*
			AddTag("Viavi_UE_id", fmt.Sprint(ueMetric.Viavi_UE_id)).
			AddTag("Viavi_Cell_id", fmt.Sprint(ueMetric.Viavi_Cell_id)).
			AddTag("Viavi_Slice_id", fmt.Sprint(ueMetric.Viavi_Slice_id)).
			AddField("DRB_UEThpDl", ueMetric.DRB_UEThpDl).
			AddField("DRB_UEThpUl", ueMetric.DRB_UEThpUl).
			AddField("RRU_PrbUsedDl", ueMetric.RRU_PrbUsedDl).
			AddField("RRU_PrbUsedUl", ueMetric.RRU_PrbUsedUl).
			//add
			AddField("Viavi_UE_anomalies", ueMetric.Viavi_UE_anomalies).
			AddField("Viavi_UE_Rsrp", ueMetric.Viavi_UE_Rsrp).
			AddField("Viavi_UE_Rsrq", ueMetric.Viavi_UE_Rsrq).
			AddField("Viavi_UE_RsSinr", ueMetric.Viavi_UE_RsSinr).
			AddTag("Viavi_Nb1_id", fmt.Sprint(ueMetric.Viavi_Nb1_id)).
			AddField("Viavi_Nb1_Rsrp", ueMetric.Viavi_Nb1_Rsrp).
			AddTag("Viavi_Nb2_id", fmt.Sprint(ueMetric.Viavi_Nb2_id)).
			AddField("Viavi_Nb2_Rsrp", ueMetric.Viavi_Nb2_Rsrp).
			//
		*/
		// Test order
		AddTag("Viavi_UE_id", fmt.Sprint(ueMetric.Viavi_UE_id)).
		AddTag("Viavi_Cell_id", fmt.Sprint(ueMetric.Viavi_Cell_id)).
		AddTag("Viavi_Nb1_id", fmt.Sprint(ueMetric.Viavi_Nb1_id)).
		AddTag("Viavi_Nb2_id", fmt.Sprint(ueMetric.Viavi_Nb2_id)).
		AddField("DRB_UEThpDl", ueMetric.DRB_UEThpDl).
		AddField("DRB_UEThpUl", ueMetric.DRB_UEThpUl).
		AddField("RRU_PrbUsedDl", ueMetric.RRU_PrbUsedDl).
		AddField("RRU_PrbUsedUl", ueMetric.RRU_PrbUsedUl).
		AddField("Viavi_UE_Rsrp", ueMetric.Viavi_UE_Rsrp).
		AddField("Viavi_UE_Rsrq", ueMetric.Viavi_UE_Rsrq).
		AddField("Viavi_UE_RsSinr", ueMetric.Viavi_UE_RsSinr).
		AddField("Viavi_Nb1_Rsrp", ueMetric.Viavi_Nb1_Rsrp).
		AddField("Viavi_Nb2_Rsrp", ueMetric.Viavi_Nb2_Rsrp).
		AddField("Viavi_UE_anomalies", ueMetric.Viavi_UE_anomalies).
		//
		SetTime(time.Unix(0, timestamp))

	writeAPI.WritePoint(context.Background(), p)
	xapp.Logger.Info("Wrote UE Metrics to InfluxDB")
}

func (c *Control) writeCellMetrics_db(cellMetricsMap *map[string]interface{}, timestamp int64) {
	writeAPI := c.client.WriteAPIBlocking(influxDBOrganization, influxDBBucket)

	// Convert map to JSON
	cellMetricsJSON, err := json.Marshal(cellMetricsMap)
	if err != nil {
		xapp.Logger.Error("Marshal Cell Metrics failed!")
	}

	// Convert JSON to structure 'CellMetricsEntry'
	var cellMetric CellMetricsEntry
	err = json.Unmarshal(cellMetricsJSON, &cellMetric)
	if err != nil {
		xapp.Logger.Error("Unmarshal Cell Metrics failed!")
	}

	p := influxdb2.NewPointWithMeasurement("cellMetrics").
		AddTag("Viavi_GnbDuId", fmt.Sprint(cellMetric.Viavi_GnbDuId)).
		AddTag("Viavi_NrCgi", fmt.Sprint(cellMetric.Viavi_NrCgi)).
		AddTag("Viavi_NrPci", fmt.Sprint(cellMetric.Viavi_NrPci)).
		AddField("DRB_UEThpDl", cellMetric.
		_UEThpDl).
		AddField("DRB_UEThpUl", cellMetric.DRB_UEThpUl).
		AddField("RRU_PrbAvailDl", cellMetric.RRU_PrbAvailDl).
		AddField("RRU_PrbAvailUl", cellMetric.RRU_PrbAvailUl).
		AddField("RRU_PrbTotDl", cellMetric.RRU_PrbTotDl).
		AddField("RRU_PrbTotUl", cellMetric.RRU_PrbTotUl).
		AddField("RRU_PrbUsedDl", cellMetric.RRU_PrbUsedDl).
		AddField("RRU_PrbUsedUl", cellMetric.RRU_PrbUsedUl).
		SetTime(time.Unix(0, timestamp))

	writeAPI.WritePoint(context.Background(), p)
	xapp.Logger.Info("Wrote Cell Metrics to InfluxDB")
}

func create_db() {
	// ?? Support InfluxDB2???
	//Create a database named kpimon in influxDB with username and password
	xapp.Logger.Info("In create_db\n")
	url := "http://ricplt-influxdb.ricplt:8086/query?q=create%20database%20kpimon" + fmt.Sprintf("&u=%s&p=%s", influxDBUser, influxDBPassword)
	_, err := http.Post(url, "", nil)
	if err != nil {
		xapp.Logger.Error("Create database failed!")
	}
	xapp.Logger.Info("exiting create_db\n")
}

func (c *Control) create_organization_bucket() {
	// Query organization
	xapp.Logger.Info("Query organization %v", influxDBOrganization)
	org, err := c.client.OrganizationsAPI().FindOrganizationByName(context.Background(), influxDBOrganization)
	if err != nil {
		// Handle error
		xapp.Logger.Error("Failed to query organization %v, reason: %v", influxDBOrganization, err)

		// Create organization
		xapp.Logger.Info("Create organization %v", influxDBOrganization)
		org, err = c.client.OrganizationsAPI().CreateOrganizationWithName(context.Background(), influxDBOrganization)
		if err != nil {
			// Handle error
			xapp.Logger.Error("Create organization failed!", err)
			return

		} else {
			// Organization created successfully
			xapp.Logger.Info("organization %v created successfully", org.Name)
		}

	}

	// Query bucket
	xapp.Logger.Info("Query bucket %v", influxDBBucket)
	bucket, err := c.client.BucketsAPI().FindBucketByName(context.Background(), influxDBBucket)
	if err != nil {
		// Handle error
		xapp.Logger.Error("Failed to query bucket %v, reason: %v", influxDBBucket, err)

		// Create Bucket
		xapp.Logger.Info("Create bucket %v", influxDBBucket)
		bucket, err = c.client.BucketsAPI().CreateBucketWithName(context.Background(), org, influxDBBucket)
		if err != nil {
			xapp.Logger.Error("Create database failed!", err)
		} else {
			xapp.Logger.Info("bucket %v created successfully", bucket.Name)
		}
	} else {
		xapp.Logger.Info("Find bucket %v under organization %v", bucket.Name, *bucket.OrgID)
	}
}

func (c Control) xAppStartCB(d interface{}) {
	xapp.Logger.Info("In callback KPI monitor xApp ...")

	// Printed after sleep is over
	fmt.Println("Wait 10s for xApp initialization, Sleeping.....")
	time.Sleep(10 * time.Second)

	// Printed after sleep is over
	fmt.Println("Sleep Over.....")

	// New thread for controlLoop
	go c.controlLoop()

	// Create a new database in InfluxDB
	//create_db()
	c.create_organization_bucket()

	// Get eNodeB list
	nbList := c.getnbList()

	// Send subscription request to connected NodeB
	for _, nb := range nbList {
		if nb.ConnectionStatus == 1 {
			xapp.Logger.Info("Before send subscription request to %v", nb.InventoryName)
			c.sendSubscription(nb.InventoryName)
			xapp.Logger.Info("After send subscription request to %v", nb.InventoryName)
		}

	}
	fmt.Println("len of Glob_cell= ", len(Glob_cell))
	fmt.Println("Glob_cell map = ", Glob_cell)

	xapp.Logger.Info("End callback KPI monitor xApp ...")
}

func (c Control) Run() {
	// Register callback
	xapp.Logger.Info("In Run() ...")
	xapp.SetReadyCB(c.xAppStartCB, true)
	// Start xApp
	xapp.Run(c)
}
