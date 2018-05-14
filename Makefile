TARGET=tf-ebs-attach

# Always compile a new binary
.PHONY: all $(TARGET) clean

all: vendor $(TARGET)

$(TARGET):
	go build

vendor:
	glide install

clean:
	rm $(TARGET)
	rm -rf vendor
