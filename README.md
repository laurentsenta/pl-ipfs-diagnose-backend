# pl-ipfs-diagnose-backend

Admittedly weak reimplementation & extension of https://github.com/aschmahmann/ipfs-check.

Project from Protocol Labs Bootcamp.

## Usage

Build and run locally:

```
go build -o backend && ./backend
```

If you have docker and make:

```
make
```

This will start serving the server at http://localhost:3333. You may use this address as a backend
URL in the frontend part.