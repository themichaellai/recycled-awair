package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/darwin"
)

var awairServiceUUIDBle = ble.MustParse(awairServiceUUID)
var awairCharacteristicUUID4Ble = ble.MustParse(awairCharacteristicUUID4)

//func findAwair() (ble.Advertisement, error) {
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	resCh := make(chan ble.Advertisement)
//	go func() {
//		if err := ble.Scan(ctx, true, func(a ble.Advertisement) {
//			if a.LocalName() == awairDeviceNme {
//				resCh <- a
//			}
//		}, nil); err != nil {
//			panic(err)
//		}
//	}()
//
//	select {
//	case <-time.After(awairSearchTimeout):
//		return nil, fmt.Errorf("timed out while scanning for awair")
//	case a := <-resCh:
//		return a, nil
//	}
//}

func ctxWithTimeout() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), awairSearchTimeout)
	return ctx, cancel
}

func findCharacteristics(client ble.Client) (char4 *ble.Characteristic, char5 *ble.Characteristic, _ error) {
	services, err := client.DiscoverServices([]ble.UUID{awairServiceUUIDBle})
	if err != nil {
		return nil, nil, fmt.Errorf("can't discover services: %s", err)
	}
	if len(services) != 1 {
		return nil, nil, fmt.Errorf("expected 1 service, got %d", len(services))
	}

	characteristics, err := client.DiscoverCharacteristics(nil, services[0])
	if err != nil {
		return nil, nil, fmt.Errorf("can't discover characteristics: %s", err)
	}
	if len(characteristics) != 2 {
		return nil, nil, fmt.Errorf("expected 2 characteristics, got %d", len(characteristics))
	}
	if characteristics[0].UUID.Equal(awairCharacteristicUUID4Ble) {
		return characteristics[0], characteristics[1], nil
	}
	return characteristics[1], characteristics[0], nil
}

type jsonResp struct {
	obj map[string]interface{}
	arr []map[string]interface{}
}

func (r jsonResp) String() string {
	if r.obj != nil {
		bs, err := json.Marshal(r.obj)
		if err != nil {
			panic(err)
		}
		return string(bs)
	}
	if r.arr != nil {
		bs, err := json.Marshal(r.arr)
		if err != nil {
			panic(err)
		}
		return string(bs)
	}
	return "<empty>"
}

func jsonReader() (chan<- []byte, <-chan jsonResp) {
	bytesCh := make(chan []byte)
	resCh := make(chan jsonResp)
	go func() {
		var buf []byte
		for {
			bytes := <-bytesCh
			buf = append(buf, bytes...)
			var res map[string]interface{}
			if err := json.Unmarshal(buf, &res); err == nil {
				resCh <- jsonResp{obj: res}
				buf = nil
			}
			var resArr []map[string]interface{}
			if err := json.Unmarshal(buf, &resArr); err == nil {
				resCh <- jsonResp{arr: resArr}
				buf = nil
			}
		}
	}()
	return bytesCh, resCh
}

func mustJson(m map[string]interface{}) []byte {
	bs, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bs
}

func waitForResp(jsonCh <-chan jsonResp) (jsonResp, error) {
	select {
	case <-time.After(awaitRespTimeout):
		return jsonResp{}, fmt.Errorf("timed out while waiting for response")
	case res := <-jsonCh:
		return res, nil
	}
}

func runGoble() error {
	d, err := darwin.NewDevice()
	if err != nil {
		return fmt.Errorf("can't new device: %s", err)
	}
	ble.SetDefaultDevice(d)

	//fmt.Printf("Scanning for %s...\n", awairSearchTimeout)
	//if _, err := findAwair(); err != nil {
	//	return fmt.Errorf("can't find awair: %w", err)
	//}

	fmt.Println("Connecting to awair...")
	ctx, cancel := ctxWithTimeout()
	client, err := ble.Connect(ctx, func(adv ble.Advertisement) bool {
		return adv.LocalName() == awairDeviceNme
	})
	cancel()
	if err != nil {
		return fmt.Errorf("can't connect to awair: %w", err)
	}

	fmt.Println("Discovering characteristics...")
	char4, char5, err := findCharacteristics(client)
	if err != nil {
		return fmt.Errorf("can't find characteristics: %w", err)
	}

	bytesCh, jsonCh := jsonReader()
	if err := client.Subscribe(char4, false, func(req []byte) {
		bytesCh <- req
		//fmt.Printf("  %s\n", string(req))
	}); err != nil {
		return fmt.Errorf("can't subscribe to characteristic: %s", err)
	}

	json, err := waitForResp(jsonCh)
	if err != nil {
		return fmt.Errorf("can't get initial json: %v", err)
	}
	fmt.Println(json.String())

	if err := client.WriteCharacteristic(char5, mustJson(map[string]interface{}{
		"cmd":          "set_country",
		"country_code": "CN",
	}), false); err != nil {
		return fmt.Errorf("couldn't write country cmd: %w", err)
	}

	json, err = waitForResp(jsonCh)
	if err != nil {
		return fmt.Errorf("did not get set country resp: %w", err)
	}
	fmt.Println(json.String())

	fmt.Println("Setting up wifi...")
	if err := client.WriteCharacteristic(char5, mustJson(map[string]interface{}{
		"cmd": "wifi_setup",
	}), false); err != nil {
		return fmt.Errorf("couldn't write wifi setup cmd: %w", err)
	}
	json, err = waitForResp(jsonCh)
	if err != nil {
		return fmt.Errorf("did not get wifi setup resp: %w", err)
	}
	fmt.Println(json.String())

	return nil
}
