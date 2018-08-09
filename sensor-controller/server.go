package sensor_controller

import (
	"fmt"
	pb "github.com/argoproj/argo-events/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
)

func (s *SensorController) UpdateSensor(signalNotificationStream pb.SensorUpdate_UpdateSensorServer) error {
	signalNotification, err := signalNotificationStream.Recv()
	if err != nil {
		log.Errorf("failed to listen to signal notification. Err: %+v", err)
		return err
	}

	var sensorCh chan pb.SensorEvent
	s.sMux.Lock()
	if sensorCh, ok := s.sensorChs[signalNotification.Name]; !ok {
		sensorCh = make(chan pb.SensorEvent)
		s.sensorChs[signalNotification.Name] = sensorCh
	}
	s.sMux.Unlock()

	go func() {
		sensorCh <- *signalNotification
	}()
	action := <-sensorCh
	signalNotificationStream.SendMsg(action)
	return nil
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
