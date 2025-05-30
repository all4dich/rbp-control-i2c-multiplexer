/*

 To provide metrics for Prometheus, we'll use the `github.com/prometheus/client_golang/prometheus` and `github.com/prometheus/client_golang/prometheus/promhttp` libraries. These libraries allow us to define metrics (like gauges for current, voltage, and power) and expose them via an HTTP endpoint that Prometheus can scrape.

Here's how to integrate Prometheus metrics into your existing Go application:

1.  **Import necessary packages:**
    *   `net/http` for the HTTP server.
    *   `github.com/prometheus/client_golang/prometheus` for defining metrics.
    *   `github.com/prometheus/client_golang/prometheus/promhttp` for the HTTP handler that exposes metrics.

2.  **Define global Prometheus Gauges:** Gauges are suitable for values that can go up and down, such as current, voltage, and power. We'll create three `Gauge` metrics: `ina260_current_amperes`, `ina260_voltage_volts`, and `ina260_power_watts`. We'll use `promauto.NewGauge` which automatically registers the metric with the default Prometheus registry.

3.  **Update metrics in the reading loop:** Inside the infinite loop where the `INA260` sensor data is read, after successfully reading the current, voltage, and power, we'll update the corresponding Prometheus gauges using the `Set()` method.

4.  **Start an HTTP server:** In a separate goroutine, an HTTP server will be started to listen for requests on a specific port (e.g., 9090). The `/metrics` endpoint will be handled by `promhttp.Handler()`, which exposes all registered Prometheus metrics.
*/
package main
import (
	"encoding/binary" // For binary.BigEndian
	"fmt"
	"log"
	"net/http" // New import for HTTP server
	"os"
	"strconv"
	"time" // For time.Sleep

	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"

	"github.com/prometheus/client_golang/prometheus"          // New import for Prometheus metrics
	"github.com/prometheus/client_golang/prometheus/promauto" // New import for auto-registering metrics
	"github.com/prometheus/client_golang/prometheus/promhttp" // New import for HTTP handler
)

// INA260 I2C address
const ina260Address = uint16(0x40) // Default INA260 I2C address

// INA260 Register Addresses
const (
	ina260RegConfig     byte = 0x00 // Configuration Register
	ina260RegCurrent    byte = 0x01 // Current Register
	ina260RegBusVoltage byte = 0x02 // Bus Voltage Register
	ina260RegPower      byte = 0x03 // Power Register
	ina260RegManufID    byte = 0xFE // Manufacturer ID Register
	ina260RegDeviceID   byte = 0xFF // Device ID Register
)

// INA260 Scaling Factors
const (
	voltageLSB = 1.25 // mV/LSB for Bus Voltage Register
	currentLSB = 1.25 // mA/LSB for Current Register
	powerLSB   = 10.0 // mW/LSB for Power Register
)

// Define Prometheus gauges
var (
	ina260Current = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ina260_current_amperes",
		Help: "Current measured by INA260 sensor in Amperes.",
	})
	ina260Voltage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ina260_voltage_volts",
		Help: "Bus voltage measured by INA260 sensor in Volts.",
	})
	ina260Power = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ina260_power_watts",
		Help: "Power measured by INA260 sensor in Watts.",
	})
)

// readINA260Reg reads a 16-bit value from the specified INA260 register.
// The INA260 returns data in Big-Endian format.
func readINA260Reg(dev *i2c.Dev, reg byte) (uint16, error) {
	writeBuf := []byte{reg}
	readBuf := make([]byte, 2) // 16-bit (2 bytes)

	// Perform the transaction: write register address, then read 2 bytes
	if err := dev.Tx(writeBuf, readBuf); err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint16(readBuf), nil
}

func main() {
	// Initialize host and I2C bus
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	bus, err := i2creg.Open("") // Opens the default I2C bus (e.g., /dev/i2c-1 on a Raspberry Pi)
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// --- Get TCA's address as argument and assign it to tcaAddress ---
	// Get the TCA address and channel number as arguments
	var tcaAddressStr string
	var channelStr string

	if len(os.Args) < 3 {
		fmt.Println("No arguments or less than 2 arguments provided. Using default TCA address 0x70 and channel 0.")
		tcaAddressStr = "0x70"
		channelStr = "0"
	} else {
		tcaAddressStr = os.Args[1]
		channelStr = os.Args[2] // Now channel number is the second argument
	}

	tcaAddress64, err := strconv.ParseUint(tcaAddressStr, 0, 16) // 0 for auto-detection of base (0x prefix means hex)
	if err != nil {
		log.Fatalf("Invalid TCA address: %v", err)
	}
	tcaAddress := uint16(tcaAddress64)

	tca := &i2c.Dev{Bus: bus, Addr: tcaAddress}
	fmt.Printf("Using TCA9548A at address: 0x%X\n", tcaAddress) // Confirm the address being used

	// --- Select the channel the INA260 is on ---
	// Get the channel number as argument and assign it to ina260Channel variable
	channelInt, err := strconv.Atoi(channelStr)
	if err != nil {
		log.Fatalf("Invalid channel number: %v", err)
	}
	if channelInt < 0 || channelInt > 7 { // TCA9548A typically has 8 channels (0-7)
		log.Fatalf("Channel number must be between 0 and 7, got %d", channelInt)
	}
	ina260Channel := byte(channelInt)

	channelSelectionByte := byte(1 << ina260Channel)

	if err := tca.Tx([]byte{channelSelectionByte}, nil); err != nil {
		log.Fatalf("Failed to select channel %d on TCA9548A: %v", ina260Channel, err)
	}
	fmt.Printf("TCA9548A: Selected channel %d\n", ina260Channel)

	// Now, communications on 'bus' will be routed to devices on the selected channel.
	// Proceed to communicate with the INA260.
	ina260 := &i2c.Dev{Bus: bus, Addr: ina260Address}

	// Optional: Read Manufacturer ID and Device ID to verify communication with INA260
	// Expected Manufacturer ID: 0x5449 (TI), Device ID: 0x2260 (INA260)
	manufID, err := readINA260Reg(ina260, ina260RegManufID)
	if err != nil {
		log.Fatalf("Failed to read INA260 Manufacturer ID: %v", err)
	}
	deviceID, err := readINA260Reg(ina260, ina260RegDeviceID)
	if err != nil {
		log.Fatalf("Failed to read INA260 Device ID: %v", err)
	}
	fmt.Printf("INA260: Manufacturer ID: 0x%X, Device ID: 0x%X\n", manufID, deviceID)
	if manufID != 0x5449 || deviceID != 0x2260 {
		fmt.Printf("Warning: Unexpected INA260 Manufacturer ID or Device ID. Expected 0x5449/0x2260, got 0x%X/0x%X\n", manufID, deviceID)
	}

	// Start HTTP server for Prometheus metrics in a goroutine
	go func() {
		http.Handle("/metrics", promhttp.Handler()) // Handles the /metrics endpoint
		port := ":9090"
		log.Printf("Starting Prometheus metrics server on port %s", port)
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}()

	// Continuously read and display values from INA260
	fmt.Println("Reading INA260 values (Voltage, Current, Power)...")
	for {
		// Read Current (Register 0x01)
		rawCurrent, err := readINA260Reg(ina260, ina260RegCurrent)
		if err != nil {
			log.Printf("Error reading current from INA260: %v", err)
			time.Sleep(1 * time.Second) // Wait before retrying
			continue
		}

		// The Current Register (0x01) is a 16-bit two's complement signed integer.
		// `binary.BigEndian.Uint16` reads it as unsigned, so cast to `int16` to preserve sign.
		// Convert raw current (mA) to Amperes (A)
		current := float64(int16(rawCurrent)) * currentLSB / 1000.0

		// Read Voltage (Register 0x02)
		rawVoltage, err := readINA260Reg(ina260, ina260RegBusVoltage)
		if err != nil {
			log.Printf("Error reading Bus Voltage from INA260: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Convert raw voltage (mV) to Volts (V)
		voltage := float64(rawVoltage) * voltageLSB / 1000.0

		// Read Power (Register 0x03)
		rawPower, err := readINA260Reg(ina260, ina260RegPower)
		if err != nil {
			log.Printf("Error reading power from INA260: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		// Convert raw power (mW) to Watts (W)
		power := float64(rawPower) * powerLSB / 1000.0

		fmt.Printf("Voltage: %.3f V, Current: %.3f A, Power: %.3f W\n", voltage, current, power)

		// Update Prometheus gauges
		ina260Current.Set(current)
		ina260Voltage.Set(voltage)
		ina260Power.Set(power)

		time.Sleep(1 * time.Second) // Wait for 1 second before the next reading
	}
}
