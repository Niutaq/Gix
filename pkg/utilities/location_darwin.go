package utilities

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreLocation

#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>

// Global variables to store result/status
static double latitude = 0;
static double longitude = 0;
static int status = 0; // 0=pending, 1=success, 2=error, 3=timeout
static char lastError[256];

@interface LocationDelegate : NSObject <CLLocationManagerDelegate>
@end

@implementation LocationDelegate

- (void)locationManager:(CLLocationManager *)manager didUpdateLocations:(NSArray<CLLocation *> *)locations {
    CLLocation *location = [locations lastObject];
    if (location) {
        latitude = location.coordinate.latitude;
        longitude = location.coordinate.longitude;
        status = 1; // Success
    }
}

- (void)locationManager:(CLLocationManager *)manager didFailWithError:(NSError *)error {
    snprintf(lastError, sizeof(lastError), "%s (code %ld)", [error.localizedDescription UTF8String], (long)error.code);
    status = 2; // Error
}

@end

static LocationDelegate *globalDelegate = nil;
static CLLocationManager *globalManager = nil;

void startNativeLocationRequest() {
    status = 0;
    lastError[0] = '\0';

    dispatch_async(dispatch_get_main_queue(), ^{
        if (!globalDelegate) globalDelegate = [[LocationDelegate alloc] init];
        if (!globalManager) {
            globalManager = [[CLLocationManager alloc] init];
            globalManager.delegate = globalDelegate;
            globalManager.desiredAccuracy = kCLLocationAccuracyBest;
        }

        [globalManager requestWhenInUseAuthorization];
        [globalManager startUpdatingLocation];
    });
}

void stopNativeLocationRequest() {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (globalManager) {
            [globalManager stopUpdatingLocation];
        }
    });
}

double getLatitude() { return latitude; }
double getLongitude() { return longitude; }
int getStatus() { return status; }
const char* getLastError() { return lastError; }

*/
import "C"
import (
	"errors"
	"time"
)

// fetchNativeLocation uses native CoreLocation via CGO.
func fetchNativeLocation() (float64, float64, error) {
	C.startNativeLocationRequest()
	defer C.stopNativeLocationRequest()

	// Wait for status update without blocking the main thread (Go's time.Sleep is non-blocking for OS threads)
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return 0, 0, errors.New("location timeout")
		case <-ticker.C:
			switch int(C.getStatus()) {
			case 1:
				return float64(C.getLatitude()), float64(C.getLongitude()), nil
			case 2:
				return 0, 0, errors.New(C.GoString(C.getLastError()))
			}
		}
	}
}
