pfeil: *.go
	go build

.PHONY: install
install: pfeil
	go install

.PHONY: tarball
tarball: pfeil README.md example.sh
	tar -czvf pfeil.tar.gz pfeil README.md example.sh

.PHONY: clean
clean:
	rm -f pfeil *.tar.gz
