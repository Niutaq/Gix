import CoreLocation
import Foundation

class LocationDelegate: NSObject, CLLocationManagerDelegate {
    let manager = CLLocationManager()

    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
    }

    func start() {
        manager.requestWhenInUseAuthorization()
        manager.startUpdatingLocation()
    }

    func locationManager(_: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        if let location = locations.first {
            print("\(location.coordinate.latitude),\(location.coordinate.longitude)")
            exit(0)
        }
    }

    func locationManager(_: CLLocationManager, didFailWithError error: Error) {
        fputs("Error: \(error.localizedDescription)\n", stderr)
        exit(1)
    }
}

let delegate = LocationDelegate()
delegate.start()

RunLoop.main.run(until: Date(timeIntervalSinceNow: 5))
fputs("Timeout fetching location\n", stderr)
exit(1)

