# Writes the three demo logs and their answer keys while the image builds,
# so a local run needs no Go on the machine. Same seed, same bytes.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod ./
COPY loggen ./loggen
RUN mkdir /generated && go run ./loggen --all --seed 42 --out /generated

FROM busybox:1.37
COPY --from=build /generated /generated
# docker compose mounts a volume at /samples and the API reads it there.
CMD ["sh", "-c", "cp -f /generated/* /samples/"]
