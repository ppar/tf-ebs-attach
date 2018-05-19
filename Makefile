TARGET=tf-ebs-attach
.PHONY: all $(TARGET) $(TARGET).linux-amd64 clean glide-lock-hash

# Always compile a new binary, do "glide install" iff ./vendor is missing
all: vendor $(TARGET).mac $(TARGET).linux-amd64

$(TARGET).linux-amd64:
	GOOS=linux GOARGH=amd64 go build -o $(TARGET).linux-amd64

$(TARGET).mac:
	GOOS=darwin GOARGH=amd64 go build -o $(TARGET).mac

clean:
	for f in $(TARGET).linux-amd64 $(TARGET).mac ]; do [ -e "$$f" ] && rm "$$f" ; done ; true
	if [ -e vendor ]; then \
		find vendor -name .DS_Store -delete; \
		rm -rf vendor; \
	fi

# Install vendored dependencies. This should pull exactly 3 (three) packages:
# $ find  vendor/github.com -mindepth 2 -maxdepth 2
# vendor/github.com/docopt/docopt-go
# vendor/github.com/hashicorp/terraform
# vendor/github.com/mattn/go-isatty
# vendor/github.com/sergi/go-diff
# vendor/github.com/yudai/gojsondiff
# vendor/github.com/yudai/golcs
vendor:
	glide install

# Fool glide by generating a new hash from glide.yaml
glide-lock-hash:
	sed -i ''  "s/^hash: .*/hash: $$(LC_ALL=C shasum -a256 glide.yaml | awk '{print $$1}')/" glide.lock
