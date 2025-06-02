package main

import (
	"encoding/binary" // For binary.BigEndian
	"flag"
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

// Define Prometheus gauges with labels
var (
	ina260Current = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ina260_current",
		Help: "Current measured by INA260 sensor in Amperes.",
	}, []string{"hostname", "device"}) // Added labels: hostname, device
	ina260Voltage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ina260_voltage",
		Help: "Bus voltage measured by INA260 sensor in Volts.",
	}, []string{"hostname", "device"}) // Added labels: hostname, device
	ina260Power = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ina260_power",
		Help: "Power measured by INA260 sensor in Watts.",
	}, []string{"hostname", "device"}) // Added labels: hostname, device
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

func initializeI2C() (i2c.BusCloser, error) {
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize host: %w", err)
	}
	bus, err := i2creg.Open("") // Opens the default I2C bus
	if err != nil {
		return nil, fmt.Errorf("failed to open I2C bus: %w", err)
	}
	return bus, nil
}

func getDevice(bus i2c.BusCloser, tcaAddressStr string, channelStr string) (*i2c.Dev, error) {
	tcaAddress64, err := strconv.ParseUint(tcaAddressStr, 0, 16) // 0 for auto-detection of base (0x prefix means hex)
	tcaAddress := uint16(tcaAddress64)
	if err != nil {
		log.Fatalf("Invalid TCA address: %v", err)
	}

	tca := &i2c.Dev{Bus: bus, Addr: tcaAddress}
	fmt.Printf("Using TCA9548A at address: 0x%X\n", tcaAddress) // Confirm the address being used

	// Get the channel number as argument and assign it to ina260Channel variable
	channelInt, err := strconv.Atoi(channelStr)
	if err != nil {
		return nil, fmt.Errorf("invalid channel number: %w", err)
	}
	if channelInt < 0 || channelInt > 7 { // TCA9548A typically has 8 channels (0-7)
		return nil, fmt.Errorf("channel number must be between 0 and 7, got %d", channelInt)
	}
	ina260Channel := byte(channelInt)
	// Select the channel on the TCA9548A multiplexer
	channelSelectionByte := byte(1 << ina260Channel)
	if err := tca.Tx([]byte{channelSelectionByte}, nil); err != nil {
		return nil, fmt.Errorf("failed to select channel %d on TCA9548A: %w", ina260Channel, err)
	}
	fmt.Printf("TCA9548A: Selected channel %d\n", ina260Channel)

	dev := &i2c.Dev{Bus: bus, Addr: ina260Address}
	// Optionally, you can perform a quick check to see if the device responds
	if err := dev.Tx([]byte{0}, nil); err != nil {
		return nil, fmt.Errorf("failed to communicate with device at address 0x%X: %w", ina260Address, err)
	}
	return dev, nil
}

func main() {
	// set flagged arguments for TCA9548A address and channel
	tcaAddressFlag := flag.String("tca_address", "0x70", "I2C address of the TCA9548A multiplexer (default: 0x70)") // Initialize host and I2C bus
	channelFlag := flag.Int("channel", 0, "Channel number on the TCA9548A multiplexer (0-7, default: 0)")

	flag.Parse()
	bus, err := initializeI2C() // Initialize I2C bus
	defer bus.Close()           // Ensure the bus is closed when done

	// -------------------- Set Hostname Label --------------------
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Failed to get hostname: %v", err)
	}

	// --- Get TCA's address as argument and assign it to tcaAddress ---
	// Get the TCA address and channel number as arguments
	// Get the TCA address and channel number from flags
	tcaAddressStr := *tcaAddressFlag
	channelStr := strconv.Itoa(*channelFlag)

	fmt.Printf("Using TCA address: %s, Channel: %s\n", tcaAddressStr, channelStr)

	ina260, err := getDevice(bus, tcaAddressStr, channelStr)

	// -------------------- Set Device Label --------------------
	deviceLabel := fmt.Sprintf("tca9548a_%s_ch%s_ina260", tcaAddressStr, channelStr)

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

		// Update Prometheus gauges with label values
		ina260Current.WithLabelValues(hostname, deviceLabel).Set(current)
		ina260Voltage.WithLabelValues(hostname, deviceLabel).Set(voltage)
		ina260Power.WithLabelValues(hostname, deviceLabel).Set(power)

		time.Sleep(1 * time.Second) // Wait for 1 second before the next reading
	}
}
