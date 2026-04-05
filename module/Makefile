# Redis FS Module - Makefile
# Modeled after Redis vectorsets module build system

uname_S := $(shell sh -c 'uname -s 2>/dev/null || echo not')

CC = cc
CFLAGS = -O2 -Wall -Wextra -Wno-unused-parameter -g $(SAN) -std=c11
LDFLAGS = -lm $(SAN)

ifeq ($(uname_S),Linux)
    SHOBJ_CFLAGS ?= -W -Wall -fno-common -g -ggdb -std=c11 -O2
    SHOBJ_LDFLAGS ?= -shared
else
    SHOBJ_CFLAGS ?= -W -Wall -dynamic -fno-common -g -ggdb -std=c11 -O3
    SHOBJ_LDFLAGS ?= -bundle -undefined dynamic_lookup
endif

.SUFFIXES: .c .xo .so

all: fs.so

.c.xo:
	$(CC) -I. $(CFLAGS) $(SHOBJ_CFLAGS) -fPIC -c $< -o $@

fs.xo: fs.c fs.h path.h redismodule.h
path.xo: path.c path.h

fs.so: fs.xo path.xo
	$(CC) -o $@ $^ $(SHOBJ_LDFLAGS) $(LDFLAGS) -lc

clean:
	rm -f *.xo *.so

test: fs.so
	@echo "Loading module into Redis..."
	redis-cli MODULE LOAD $(PWD)/fs.so

.PHONY: all clean test
