FROM golang:1.23

WORKDIR /app
COPY . .
RUN go build -o hackathon .

EXPOSE 8080
CMD ["./hackathon"]