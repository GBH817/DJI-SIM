package engine

import (
	"sync"
	"testing"
	"time"

	"drone-sim-backend/internal/trajectory"
)

func makeTestTrajectory() *trajectory.Trajectory {
	return &trajectory.Trajectory{
		ID:   "test-drone-1",
		Name: "Test Drone",
		Waypoints: []trajectory.Waypoint{
			{
				Index:           0,
				Longitude:       116.397428,
				Latitude:        39.909204,
				Height:          100,
				EllipsoidHeight: 100,
				Speed:           10,
				Heading:         90,
			},
			{
				Index:           1,
				Longitude:       116.407428,
				Latitude:        39.909204,
				Height:          150,
				EllipsoidHeight: 150,
				Speed:           10,
				Heading:         90,
			},
			{
				Index:           2,
				Longitude:       116.407428,
				Latitude:        39.919204,
				Height:          200,
				EllipsoidHeight: 200,
				Speed:           10,
				Heading:         0,
			},
		},
		TakeOffPoint:    [3]float64{39.909204, 116.397428, 100},
		TotalDistance:   2000,
		TotalDuration:   200 * time.Second,
		AutoFlightSpeed: 10,
	}
}

func TestEngineAddDrone(t *testing.T) {
	engine := NewEngine(nil)

	traj := makeTestTrajectory()
	droneID := engine.AddDrone(traj)

	if droneID != traj.ID {
		t.Errorf("expected droneID %q, got %q", traj.ID, droneID)
	}

	status := engine.GetStatus()
	ds, ok := status[droneID]
	if !ok {
		t.Fatal("drone not found in status map")
	}

	if ds.Status != "idle" {
		t.Errorf("expected status idle, got %q", ds.Status)
	}

	if ds.Name != traj.Name {
		t.Errorf("expected name %q, got %q", traj.Name, ds.Name)
	}

	if ds.Progress != 0 {
		t.Errorf("expected progress 0, got %f", ds.Progress)
	}
}

func TestEngineStartStop(t *testing.T) {
	var mu sync.Mutex
	var receivedTelemetry []trajectory.RemoteIDTelemetry

	callback := func(tm trajectory.RemoteIDTelemetry) {
		mu.Lock()
		receivedTelemetry = append(receivedTelemetry, tm)
		mu.Unlock()
	}

	engine := NewEngine(callback)
	traj := makeTestTrajectory()
	droneID := engine.AddDrone(traj)

	// 启动仿真
	err := engine.Start(droneID)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// 等待一小段时间让 ticker 触发几次
	time.Sleep(500 * time.Millisecond)

	// 停止仿真
	err = engine.Stop(droneID)
	if err != nil {
		t.Fatalf("failed to stop: %v", err)
	}

	// 等待 goroutine 退出
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(receivedTelemetry)
	mu.Unlock()

	if count == 0 {
		t.Error("expected at least some telemetry callbacks, got none")
	}

	t.Logf("received %d telemetry callbacks", count)

	// 验证状态已变为 idle
	status := engine.GetStatus()
	ds, ok := status[droneID]
	if !ok {
		t.Fatal("drone not found in status map after stop")
	}

	if ds.Status != "idle" {
		t.Errorf("expected status idle after stop, got %q", ds.Status)
	}
}

func TestAddDroneMultiple(t *testing.T) {
	engine := NewEngine(nil)

	traj1 := makeTestTrajectory()
	traj1.ID = "drone-a"
	traj1.Name = "Drone A"

	traj2 := makeTestTrajectory()
	traj2.ID = "drone-b"
	traj2.Name = "Drone B"

	id1 := engine.AddDrone(traj1)
	id2 := engine.AddDrone(traj2)

	status := engine.GetStatus()
	if len(status) != 2 {
		t.Errorf("expected 2 drones in status, got %d", len(status))
	}

	if id1 != "drone-a" || id2 != "drone-b" {
		t.Errorf("unexpected drone ids: %q, %q", id1, id2)
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	engine := NewEngine(nil)
	traj := makeTestTrajectory()
	droneID := engine.AddDrone(traj)

	err := engine.Start(droneID)
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	// 再次启动应该报错
	err = engine.Start(droneID)
	if err == nil {
		t.Error("expected error when starting already running drone")
	}

	engine.Stop(droneID)
	time.Sleep(200 * time.Millisecond)
}

func TestStopIdleDrone(t *testing.T) {
	engine := NewEngine(nil)
	traj := makeTestTrajectory()
	droneID := engine.AddDrone(traj)

	// 停止 idle 的 drone 应该不报错
	err := engine.Stop(droneID)
	if err != nil {
		t.Errorf("stopping idle drone should not error: %v", err)
	}
}

func TestPauseResume(t *testing.T) {
	var mu sync.Mutex
	var telemetryStatuses []string

	callback := func(tm trajectory.RemoteIDTelemetry) {
		mu.Lock()
		telemetryStatuses = append(telemetryStatuses, tm.Status)
		mu.Unlock()
	}

	engine := NewEngine(callback)
	traj := makeTestTrajectory()
	droneID := engine.AddDrone(traj)

	err := engine.Start(droneID)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// 暂停
	err = engine.Pause(droneID)
	if err != nil {
		t.Fatalf("failed to pause: %v", err)
	}

	// 等待一小段时间，确保不会再有遥测
	time.Sleep(300 * time.Millisecond)

	// 恢复
	err = engine.Resume(droneID)
	if err != nil {
		t.Fatalf("failed to resume: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	engine.Stop(droneID)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	t.Logf("received %d telemetry status updates", len(telemetryStatuses))
	mu.Unlock()
}
