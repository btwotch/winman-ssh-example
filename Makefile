winman-ssh: *.go
	go fmt $^
	goimports -w $^
	go build -o winman-ssh $^

clean:
	rm -fv *.log
