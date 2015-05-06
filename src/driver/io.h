#ifndef __INCLUDE_IO_H__
#define __INCLUDE_IO_H__

// Returns 0 on init failure
int io_init();

void io_set_bit(int channel);
void io_clear_bit(int channel);
void io_write_analog(int channel, int value);
int io_read_bit(int channel);
int io_read_analog(int channel);

#endif
