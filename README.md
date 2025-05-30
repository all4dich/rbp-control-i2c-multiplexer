# Raspberry PI I2C Multiplexer
## Integrate INA260 power monitor with TCA9548A I2C bus multiplexer

This Go program demonstrates a common and crucial use case for integrating an INA260 precision digital power monitor with a TCA9548A I2C bus multiplexer.

Here's an explanation of the use case and the role of each component:

### INA260 Precision Digital Power Monitor

* **What it is:** The `INA260` is a high-precision digital power monitor with an I2C interface. It integrates a current sense amplifier, a high-resolution analog-to-digital converter (ADC), and a precision internal shunt resistor.
* **Purpose:** Its primary function is to accurately measure voltage, current, and calculate power consumption of a circuit or device connected to it. It provides these readings directly in digital form over I2C, simplifying data acquisition. The code reads current, voltage, and power registers (`0x01`, `0x02`, `0x03`) and applies scaling factors to get values in Amperes, Volts, and Watts.

### TCA9548A I2C Bus Multiplexer

* **What it is:** The TCA9548A is an 8-channel I2C bus switch (multiplexer). It allows a single I2C master device (like a Raspberry Pi or other microcontroller) to communicate with up to eight independent I2C slave devices, or groups of slave devices, that might share the same I2C address.
* **Purpose:** The standard I2C protocol allows multiple slave devices to share the same bus, but each slave device must have a unique I2C address. When you have multiple identical sensors (like several INA260s) that all share the same default I2C address (e.g., `0x40` for INA260), you cannot connect them directly to the same I2C bus. The TCA9548A solves this by acting as a traffic director. You communicate with the TCA9548A to select one of its eight downstream I2C channels, and then any subsequent I2C communication from the master is routed only to the devices on the selected channel.

### Integration Use Case: Monitoring Multiple Power Rails

The primary use case for integrating the INA260 with the TCA9548A, as demonstrated in the Go code, is **to monitor power consumption from multiple, independent power rails or circuits that would otherwise use identical INA260 sensors and thus conflict on a single I2C bus.**

**Here's how it works and its benefits:**

1. **Addressing Conflict Resolution:** Imagine you have a system with several sub-circuits or components (e.g., CPU, GPU, memory, peripherals) and you want to measure the power consumption of each independently using an INA260. Since all INA260s typically have a default I2C address (`0x40`), you can't put them all on the same primary I2C bus.
2. **Multiplexed Access:** The TCA9548A acts as a gateway. Each INA260 sensor is connected to a *different* channel of the TCA9548A.
3. **Sequential Reading:**
   * The Go program first initializes the main I2C bus and communicates with the TCA9548A (e.g., at address `0x70`).
   * It then writes a specific byte to the TCA9548A to *select* one of its eight channels (e.g., channel 0).
   * Once a channel is selected, the main I2C bus effectively connects directly to the devices on that channel. The program can then read from the INA260 sensor attached to that selected channel.
   * To read from another INA260, the program would again communicate with the TCA9548A to select a *different* channel, and then read from the INA260 on that new channel.
4. **Scalability and Resource Optimization:** This setup allows you to expand the number of devices you can monitor beyond the limitation of unique I2C addresses on a single bus. Instead of requiring a separate I2C bus controller for each INA260, you can use a single I2C bus and one TCA9548A to manage up to eight INA260s (or other I2C devices with conflicting addresses).
5. **Data Collection and Monitoring (Prometheus):** The Go code further enhances this by:
   * Continuously reading voltage, current, and power from the selected INA260.
   * Exposing these metrics via a Prometheus HTTP endpoint (`/metrics`). This allows the system to be integrated into a larger monitoring infrastructure (like Prometheus + Grafana) for real-time visualization and alerting on power consumption data.

In essence, the TCA9548A allows the single master to "switch" between multiple identical INA260 sensors, enabling comprehensive power monitoring across different parts of a system using a single I2C interface.



## Provide metrics for Prometheus Server

To provide metrics for Prometheus, we'll use the `github.com/prometheus/client_golang/prometheus` and `github.com/prometheus/client_golang/prometheus/promhttp` libraries. These libraries allow us to define metrics (like gauges for current, voltage, and power) and expose them via an HTTP endpoint that Prometheus can scrape.

Here's how to integrate Prometheus metrics into your existing Go application:

1. **Import necessary packages:**
   * `net/http` for the HTTP server.
   * `github.com/prometheus/client_golang/prometheus` for defining metrics.
   * `github.com/prometheus/client_golang/prometheus/promhttp` for the HTTP handler that exposes metrics.

2. **Define global Prometheus Gauges:** Gauges are suitable for values that can go up and down, such as current, voltage, and power. We'll create three `Gauge` metrics: `ina260_current_amperes`, `ina260_voltage_volts`, and `ina260_power_watts`. We'll use `promauto.NewGauge` which automatically registers the metric with the default Prometheus registry.

3. **Update metrics in the reading loop:** Inside the infinite loop where the `INA260` sensor data is read, after successfully reading the current, voltage, and power, we'll update the corresponding Prometheus gauges using the `Set()` method.

4. **Start an HTTP server:** In a separate goroutine, an HTTP server will be started to listen for requests on a specific port (e.g., 9090). The `/metrics` endpoint will be handled by `promhttp.Handler()`, which exposes all registered Prometheus metrics.
