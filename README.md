# Reused Mobile Devices

Server and Client for running an automatically managed llama RPC cluster.

## Run Server

```sh
go run .
```

## Run Client

The client will announce itself to the tracker and run the rpc server.
Replace `/path/to/rpc-server` with the path to the rpc server binary.
Replace `127.0.0.1:4917` with the ip and port of the tracker.

```sh
go run ./cmd/linux-client/ -cmd /path/to/rpc-server -tracker 127.0.0.1:4917 -- -c
```

## Run Android Client

1. Open `llama-rpc-app` in Android Studio or build it via Gradle (`./gradlew assembleDebug`).
2. Install the APK to your device.
3. Open the app, set the `Host` to `0.0.0.0`, port to your desired RPC port (e.g., `50052`), and the **Discovery IP** and **Port** to match the tracker server.
4. Click **Start Server**. The client will announce itself to the tracker using HTTP GET and automatically manage its sleep intervals.

## Run iOS Client

1. Open `llama-rpc-app-ios/distributed-ml-ggml-client-ios.xcodeproj` in Xcode.
2. Build and run the app on your physical iOS device.
3. Use the `startRPCServer` API via the UI or programmatically, providing the tracker's `discoveryIp` and `discoveryPort`.
4. The iOS device will begin making HTTP GET `/announce` requests to register with the tracker.

