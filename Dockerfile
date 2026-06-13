FROM golang:1.25

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    nodejs \
    bash \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .
RUN go build -o hackathon .

EXPOSE 8080
CMD ["./hackathon"]