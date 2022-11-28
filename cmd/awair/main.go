package main

import (
	"time"
)

const awairDeviceNme = "AWAIR-R2"
const awairSearchTimeout = time.Second * 15

const awairServiceUUID = "2f2dfff0-2e85-649d-3545-3586428f5da3"
const awairCharacteristicUUID4 = "2f2dfff4-2e85-649d-3545-3586428f5da3"
const awairCharacteristicUUID5 = "2f2dfff5-2e85-649d-3545-3586428f5da3"

func main() {
	if err := runGoble(); err != nil {
		panic(err)
	}
	//if err := runTinygo(); err != nil {
	//	panic(err)
	//}
}
