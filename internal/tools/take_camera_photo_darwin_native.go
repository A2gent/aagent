//go:build darwin && cgo

package tools

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework AppKit

#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <AppKit/AppKit.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

@interface AAgentPhotoCaptureDelegate : NSObject <AVCapturePhotoCaptureDelegate>
@property (atomic, strong) NSData *capturedData;
@property (atomic, strong) NSError *capturedError;
@property (nonatomic) dispatch_semaphore_t semaphore;
@end

@implementation AAgentPhotoCaptureDelegate
- (void)photoOutput:(AVCapturePhotoOutput *)output
didFinishProcessingPhoto:(AVCapturePhoto *)photo
              error:(NSError *)error {
    if (error != nil) {
        self.capturedError = error;
    } else {
        self.capturedData = [photo fileDataRepresentation];
        if (self.capturedData == nil) {
            self.capturedError = [NSError errorWithDomain:@"aagent.camera"
                                                     code:500
                                                 userInfo:@{NSLocalizedDescriptionKey: @"failed to encode photo data"}];
        }
    }
    dispatch_semaphore_signal(self.semaphore);
}
@end

static void set_error(char **err_out, NSString *message) {
    if (err_out == NULL) {
        return;
    }
    const char *utf8 = [message UTF8String];
    if (utf8 == NULL) {
        utf8 = "unknown error";
    }
    *err_out = strdup(utf8);
}

int aagent_capture_photo_darwin(int camera_index, const char *output_path, const char *format, char **err_out) {
    @autoreleasepool {
        if (camera_index <= 0) {
            camera_index = 1;
        }
        if (output_path == NULL || format == NULL) {
            set_error(err_out, @"invalid capture arguments");
            return 1;
        }

        NSString *outputPath = [NSString stringWithUTF8String:output_path];
        NSString *formatStr = [[NSString stringWithUTF8String:format] lowercaseString];
        if (outputPath == nil || formatStr == nil) {
            set_error(err_out, @"invalid capture arguments encoding");
            return 1;
        }
        if (![formatStr isEqualToString:@"jpg"] &&
            ![formatStr isEqualToString:@"jpeg"] &&
            ![formatStr isEqualToString:@"png"]) {
            set_error(err_out, [NSString stringWithFormat:@"unsupported format: %@", formatStr]);
            return 1;
        }

        AVAuthorizationStatus auth = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
        if (auth == AVAuthorizationStatusNotDetermined) {
            dispatch_semaphore_t authSem = dispatch_semaphore_create(0);
            __block BOOL granted = NO;
            [AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL ok) {
                granted = ok;
                dispatch_semaphore_signal(authSem);
            }];
            dispatch_semaphore_wait(authSem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(10 * NSEC_PER_SEC)));
            if (!granted) {
                set_error(err_out, @"camera access was denied");
                return 1;
            }
            auth = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
        }
        if (auth != AVAuthorizationStatusAuthorized) {
            set_error(err_out, @"camera access is not authorized for this process");
            return 1;
        }

        AVCaptureDeviceDiscoverySession *discovery =
            [AVCaptureDeviceDiscoverySession discoverySessionWithDeviceTypes:@[
                AVCaptureDeviceTypeBuiltInWideAngleCamera,
                AVCaptureDeviceTypeExternalUnknown
            ]
            mediaType:AVMediaTypeVideo
            position:AVCaptureDevicePositionUnspecified];

        NSArray<AVCaptureDevice *> *devices = [discovery devices];
        if (devices.count == 0) {
            set_error(err_out, @"no camera devices found");
            return 1;
        }
        if (camera_index > (int)devices.count) {
            set_error(err_out, [NSString stringWithFormat:@"camera_index out of range: %d (available: %lu)",
                                camera_index, (unsigned long)devices.count]);
            return 1;
        }

        AVCaptureDevice *device = devices[(NSUInteger)(camera_index - 1)];
        NSError *captureErr = nil;
        AVCaptureDeviceInput *input = [AVCaptureDeviceInput deviceInputWithDevice:device error:&captureErr];
        if (input == nil || captureErr != nil) {
            NSString *msg = captureErr != nil ? captureErr.localizedDescription : @"unable to create camera input";
            set_error(err_out, msg);
            return 1;
        }

        AVCaptureSession *session = [[AVCaptureSession alloc] init];
        [session beginConfiguration];
        if (![session canAddInput:input]) {
            [session commitConfiguration];
            set_error(err_out, @"unable to add camera input");
            return 1;
        }
        [session addInput:input];

        AVCapturePhotoOutput *photoOutput = [[AVCapturePhotoOutput alloc] init];
        if (![session canAddOutput:photoOutput]) {
            [session commitConfiguration];
            set_error(err_out, @"unable to add photo output");
            return 1;
        }
        [session addOutput:photoOutput];
        [session commitConfiguration];

        AAgentPhotoCaptureDelegate *delegate = [[AAgentPhotoCaptureDelegate alloc] init];
        delegate.semaphore = dispatch_semaphore_create(0);

        AVCapturePhotoSettings *settings = [AVCapturePhotoSettings photoSettings];
        [session startRunning];
        [NSThread sleepForTimeInterval:0.3];
        [photoOutput capturePhotoWithSettings:settings delegate:delegate];

        long semResult = dispatch_semaphore_wait(delegate.semaphore, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(12 * NSEC_PER_SEC)));
        [session stopRunning];

        if (semResult != 0) {
            set_error(err_out, @"camera capture timed out");
            return 1;
        }
        if (delegate.capturedError != nil) {
            set_error(err_out, delegate.capturedError.localizedDescription);
            return 1;
        }
        if (delegate.capturedData == nil || delegate.capturedData.length == 0) {
            set_error(err_out, @"no image data captured");
            return 1;
        }

        NSData *finalData = delegate.capturedData;
        if ([formatStr isEqualToString:@"png"]) {
            NSImage *image = [[NSImage alloc] initWithData:delegate.capturedData];
            NSData *tiffData = [image TIFFRepresentation];
            NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiffData];
            NSData *pngData = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            if (pngData == nil || pngData.length == 0) {
                set_error(err_out, @"failed to convert captured image to png");
                return 1;
            }
            finalData = pngData;
        }

        NSURL *url = [NSURL fileURLWithPath:outputPath];
        NSError *writeErr = nil;
        BOOL ok = [finalData writeToURL:url options:NSDataWritingAtomic error:&writeErr];
        if (!ok) {
            NSString *msg = writeErr != nil ? writeErr.localizedDescription : @"failed to write output image";
            set_error(err_out, msg);
            return 1;
        }
        return 0;
    }
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func captureCameraPhotoDarwin(cameraIndex int, format string, outputPath string) error {
	cOutputPath := C.CString(outputPath)
	cFormat := C.CString(format)
	defer C.free(unsafe.Pointer(cOutputPath))
	defer C.free(unsafe.Pointer(cFormat))

	var cErr *C.char
	rc := C.aagent_capture_photo_darwin(C.int(cameraIndex), cOutputPath, cFormat, &cErr)
	if rc == 0 {
		return nil
	}
	defer func() {
		if cErr != nil {
			C.free(unsafe.Pointer(cErr))
		}
	}()
	if cErr != nil {
		return fmt.Errorf("native camera capture failed: %s", C.GoString(cErr))
	}
	return fmt.Errorf("native camera capture failed")
}
