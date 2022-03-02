all:
	docker build -t pl-ipfs-diagnose-backend . && docker run -p 3333:3333 -it pl-ipfs-diagnose-backend
