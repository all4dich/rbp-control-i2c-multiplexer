package main

import (
    "fmt"
    "log"

    "periph.io/x/conn/v3/i2c"
    "periph.io/x/conn/v3/i2c/i2creg"
    "periph.io/x/host/v3"
)

func main() {
    // Initialize host and I2C bus
    if _, err := host.Init(); err != nil {
        log.Fatal(err)
    }

    bus, err := i2creg.Open("") // Opens the default I2C bus
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    tcaAddress := uint16(0x70) // Default TCA9548A address
    tca := &i2c.Dev{Bus: bus, Addr: tcaAddress}

    // --- Select the channel the INA260 is on ---
    // For example, if INA260 is on channel 3 of the multiplexer:
    ina260Channel := byte(0)
    channelSelectionByte := byte(1 << ina260Channel) // 0x08 for channel 3

    if err := tca.Tx([]byte{channelSelectionByte}, nil); err != nil {
        log.Fatalf("Failed to select channel %d on TCA9548A: %v", ina260Channel, err)
    }
    fmt.Printf("TCA9548A: Selected channel %d\n", ina260Channel)

    // Now, communications on 'bus' will be routed to devices on the selected channel.
    // Proceed to communicate with the INA260.
    // ... (INA260 communication code follows)
}
