package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/darwin"
)

var awairServiceUUIDBle = ble.MustParse(awairServiceUUID)
var awairCharacteristicUUID4Ble = ble.MustParse(awairCharacteristicUUID4)
var awairCharacteristicUUID5Ble = ble.MustParse(awairCharacteristicUUID5)

func findAwair() (ble.Advertisement, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resCh := make(chan ble.Advertisement)
	go func() {
		ble.Scan(ctx, true, func(a ble.Advertisement) {
			if a.LocalName() == awairDeviceNme {
				resCh <- a
			}
		}, nil)
	}()

	select {
	case <-time.After(awairSearchTimeout):
		return nil, fmt.Errorf("timed out while scanning for awair")
	case a := <-resCh:
		return a, nil
	}
}

func ctxWithTimeout() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), awairSearchTimeout)
	return ctx
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

func runGoble() error {
	d, err := darwin.NewDevice()
	if err != nil {
		return fmt.Errorf("can't new device: %s", err)
	}
	ble.SetDefaultDevice(d)

	//fmt.Printf("Scanning for %s...\n", awairSearchTimeout)
	//if _, err := findAwair(); err != nil {
	//	return fmt.Errorf("can't find awair: %s", err)
	//}

	fmt.Println("Connecting to awair...")
	client, err := ble.Connect(ctxWithTimeout(), func(adv ble.Advertisement) bool {
		return adv.LocalName() == awairDeviceNme
	})
	if err != nil {
		return fmt.Errorf("can't connect to awair: %s", err)
	}

	fmt.Println("Discovering characteristics...")
	char4, char5, err := findCharacteristics(client)
	if err != nil {
		return fmt.Errorf("can't find characteristics: %s", err)
	}
	fmt.Printf("%#v %#v\n", char4, char5)
	//char4.HandleWrite
	client.Subscribe(char4, false, func(req []byte) {
		fmt.Printf("char4: %s\n", string(req))
	})
	fmt.Println("Subscribed to char4...")
	<-time.After(10 * time.Second)

	return nil
}
