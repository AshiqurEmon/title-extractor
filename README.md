# title-extractor
# installation
clone the repo
go build -o titlextractor titlextractor.go
mv titlextractor /usr/local/bin

# Usage
cat urls.txt|titlextractor -f -c 5|tee -a output.txt
