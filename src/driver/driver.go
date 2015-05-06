package driver
/*
#cgo LDFLAGS: -lpthread -lcomedi -lm
#include "io.h"
*/
import ("C")
import("errors")

func IOInit() error {
	if err := int(C.io_init()); err == 0 {
		return errors.New("Could not initialise Comedy!")
	}
	return nil
}

func IO_set_bit(channel int) {
	C.io_set_bit(C.int(channel))
}

func IO_clear_bit(channel int) {
	C.io_clear_bit(C.int(channel))
}

func IO_write_analog(channel, value int) {
	C.io_write_analog(C.int(channel), C.int(value))
}

func IO_read_bit(channel int) bool {
	return bool(int(C.io_read_bit(C.int(channel))) != 0)
}

func IO_read_analog(channel int) int {
	return int(C.io_read_analog(C.int(channel)))
}
