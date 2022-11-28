package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/darwin"
)

var awairServiceUUIDBle = ble.MustParse(awairServiceUUID)
var awairCharacteristicUUID4Ble = ble.MustParse(awairCharacteristicUUID4)

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

var errWaitForResp = fmt.Errorf("timed out while waiting for response")

func waitForResp(jsonCh <-chan jsonResp) (jsonResp, error) {
	return waitForRespWithTimeout(jsonCh, awaitRespTimeout)
}

func waitForRespWithTimeout(jsonCh <-chan jsonResp, timeout time.Duration) (jsonResp, error) {
	select {
	case <-time.After(timeout):
		return jsonResp{}, errWaitForResp
	case res := <-jsonCh:
		return res, nil
	}
}

func sendSimpleRequest(
	client ble.Client,
	char5 *ble.Characteristic,
	jsonCh <-chan jsonResp,
	req map[string]interface{},
) error {
	if err := client.WriteCharacteristic(char5, mustJson(req), false); err != nil {
		return fmt.Errorf("couldn't send request: %s", err)
	}
	json, err := waitForResp(jsonCh)
	if err != nil {
		return fmt.Errorf("could not get response: %s", err)
	}
	fmt.Println(json.String())
	return nil
}

func runGoble() error {
	d, err := darwin.NewDevice()
	if err != nil {
		return fmt.Errorf("can't new device: %s", err)
	}
	ble.SetDefaultDevice(d)

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

	if err := sendSimpleRequest(client, char5, jsonCh, map[string]interface{}{
		"cmd":          "set_country",
		"country_code": "CN",
	}); err != nil {
		return fmt.Errorf("couldn't set country: %w", err)
	}

	fmt.Println("Setting up wifi...")
	if err := sendSimpleRequest(client, char5, jsonCh, map[string]interface{}{
		"cmd": "wifi_setup",
	}); err != nil {
		return fmt.Errorf("couldn't set up wifi: %w", err)
	}

	fmt.Println("Connecting to network...")
	if err := client.WriteCharacteristic(char5, mustJson(map[string]interface{}{
		"SSID":     "",
		"password": "",
		"security": "WPA2 AES PSK",
	}), false); err != nil {
		return fmt.Errorf("couldn't connect to wifi: %w", err)
	}
	for {
		json, err = waitForRespWithTimeout(jsonCh, 30*time.Second)
		if err != nil {
			if errors.Is(err, errWaitForResp) {
				return fmt.Errorf("timed out waiting for wifi response")
			}
			return fmt.Errorf("could not get wifi connect resp: %w", err)
		}
		fmt.Println(json.String())
		if json.obj["state"] == "OK" {
			break
		}
	}

	fmt.Println("Doing connection test...")
	if err := sendSimpleRequest(client, char5, jsonCh, map[string]interface{}{
		"cmd": "connection_test",
	}); err != nil {
		return fmt.Errorf("could not do connection test: %w", err)
	}

	fmt.Println("Registering device...")
	if err := sendSimpleRequest(client, char5, jsonCh, map[string]interface{}{
		"cmd": "device_register",
	}); err != nil {
		return fmt.Errorf("could not register device: %w", err)
	}

	fmt.Println("Setting mqtt token...")
	if err := sendSimpleRequest(client, char5, jsonCh, map[string]interface{}{
		"cmd":        "set_mqtt_token",
		"mqtt_token": "", // TODO: JWT pulled from /register-device endpoint
	}); err != nil {
		return fmt.Errorf("could not register device: %w", err)
	}

	return nil
}
