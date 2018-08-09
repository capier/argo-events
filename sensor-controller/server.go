package sensor_controller

import (
	"context"
	"fmt"
	pb "github.com/argoproj/argo-events/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
)

func (s *SensorController) UpdateSensor(ctx context.Context, se *pb.SensorEvent) (*pb.SensorResponse, error) {
	defer s.sMux.Unlock()
	s.sMux.Lock()

	var sensorCh chan pb.SensorEvent
	if sensorCh, ok := s.sensorChs[se.Name]; !ok {
		sensorCh = make(chan pb.SensorEvent)
		s.sensorChs[se.Name] = sensorCh
	}

	go func() {
		sensorCh <- *se
	}()
	action := <-sensorCh
	return &pb.SensorResponse{
		Action: action.Name,
	}, nil
}

func (s *SensorController) startServer(port int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Errorf("failed starting gRPC server. Err: %+v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterSensorUpdateServer(grpcServer, s)
	err = grpcServer.Serve(lis)
}
