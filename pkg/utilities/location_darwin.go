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

void fetchLocationBridge(int timeoutSeconds) {
    status = 0;
    lastError[0] = '\0';

    dispatch_sync(dispatch_get_main_queue(), ^{
        static LocationDelegate *delegate = nil;
        if (!delegate) delegate = [[LocationDelegate alloc] init];

        CLLocationManager *manager = [[CLLocationManager alloc] init];
        manager.delegate = delegate;
        manager.desiredAccuracy = kCLLocationAccuracyBest;

        printf("[CGO] Requesting location (timeout: %d)...\n", timeoutSeconds);
        [manager requestWhenInUseAuthorization];
        [manager startUpdatingLocation];

        NSDate *timeoutDate = [NSDate dateWithTimeIntervalSinceNow:timeoutSeconds];
        while (status == 0 && [timeoutDate timeIntervalSinceNow] > 0) {
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
        }

        if (status == 0) {
            printf("[CGO] Timeout reached.\n");
            status = 3;
        } else if (status == 1) {
            printf("[CGO] Success!\n");
        } else {
            printf("[CGO] Error occurred.\n");
        }

        [manager stopUpdatingLocation];
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
	"fmt"
)

// fetchNativeLocation uses native CoreLocation via CGO.
func fetchNativeLocation() (float64, float64, error) {
	C.fetchLocationBridge(20)

	status := int(C.getStatus())
	if status == 1 {
		return float64(C.getLatitude()), float64(C.getLongitude()), nil
	} else if status == 2 {
		return 0, 0, fmt.Errorf("core location error: %v", C.GoString(C.getLastError()))
	} else if status == 3 {
		return 0, 0, errors.New("core location timeout")
	}

	return 0, 0, fmt.Errorf("unknown location status: %d", status)
}
