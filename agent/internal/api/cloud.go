package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime"
	"time"

	cloudclient "github.com/AaronChou313/open-remote-net/agent/internal/cloud"
	"github.com/AaronChou313/open-remote-net/agent/internal/state"
)

func (s *Server) publicCloud() map[string]any {
	s.cloudMu.RLock()
	defer s.cloudMu.RUnlock()
	value := s.Config.Cloud
	result := map[string]any{"bound": value.Bound, "base_url": value.BaseURL, "error": s.cloudError}
	if !value.Bound && value.UserCode != "" {
		result["pending"] = map[string]any{"user_code": value.UserCode, "verification_url": value.VerificationURL}
	}
	return result
}
func (s *Server) cloudStatus(w http.ResponseWriter, r *http.Request) { reply(w, s.publicCloud()) }
func (s *Server) metadata() map[string]any {
	s.cloudMu.RLock()
	cfg := s.Config
	s.cloudMu.RUnlock()
	return map[string]any{"instance_id": cfg.Cloud.DeviceID, "name": cfg.Hostname, "os": runtime.GOOS, "addresses": []string{}, "port": 4789, "kind": "orn"}
}
func (s *Server) cloudBind(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CloudURL string `json:"cloud_url"`
	}
	if err := body(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	base, err := cloudclient.Base(input.CloudURL)
	if err != nil {
		fail(w, 400, err)
		return
	}
	s.cloudMu.Lock()
	if s.Config.Cloud.Bound {
		s.cloudMu.Unlock()
		fail(w, 409, http.ErrUseLastResponse)
		return
	}
	if s.Config.Cloud.DeviceID == "" {
		var id [16]byte
		rand.Read(id[:])
		id[6] = (id[6] & 15) | 64
		id[8] = (id[8] & 63) | 128
		value := hex.EncodeToString(id[:])
		s.Config.Cloud.DeviceID = value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
	}
	s.cloudMu.Unlock()
	var result struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if err = cloudclient.Call(r.Context(), base, "/agent/bindings/start", "", s.metadata(), &result); err != nil {
		fail(w, 503, err)
		return
	}
	s.cloudMu.Lock()
	s.Config.Cloud.BaseURL = base
	s.Config.Cloud.DeviceToken = result.DeviceCode
	s.Config.Cloud.UserCode = result.UserCode
	s.Config.Cloud.VerificationURL = result.VerificationURL
	s.Config.Cloud.Expires = float64(time.Now().Unix() + int64(result.ExpiresIn))
	err = state.Save(s.ConfigPath, s.Config)
	s.cloudMu.Unlock()
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, s.publicCloud())
}
func (s *Server) syncCloud(ctx context.Context) error {
	s.cloudMu.RLock()
	cfg := s.Config
	s.cloudMu.RUnlock()
	if cfg.Cloud.DeviceToken == "" {
		return nil
	}
	if !cfg.Cloud.Bound {
		var result struct {
			Status string `json:"status"`
		}
		if err := cloudclient.Call(ctx, cfg.Cloud.BaseURL, "/agent/bindings/poll", "", map[string]string{"device_code": cfg.Cloud.DeviceToken}, &result); err != nil {
			return err
		}
		if result.Status != "approved" {
			return nil
		}
		s.cloudMu.Lock()
		s.Config.Cloud.Bound = true
		cfg = s.Config
		err := state.Save(s.ConfigPath, s.Config)
		s.cloudMu.Unlock()
		if err != nil {
			return err
		}
	}
	status, err := s.Node.Status(ctx)
	if err != nil {
		return err
	}
	if status.BackendState == "NeedsLogin" || status.BackendState == "NoState" {
		var enrollment struct {
			Key        string `json:"key"`
			ControlURL string `json:"control_url"`
		}
		if err = cloudclient.Call(ctx, cfg.Cloud.BaseURL, "/agent/preauth", cfg.Cloud.DeviceToken, map[string]any{}, &enrollment); err != nil {
			return err
		}
		if err = s.Node.Login(ctx, enrollment.Key); err != nil {
			return err
		}
	}
	var heartbeat map[string]any
	if err = cloudclient.Call(ctx, cfg.Cloud.BaseURL, "/agent/heartbeat", cfg.Cloud.DeviceToken, s.metadata(), &heartbeat); err != nil {
		return err
	}
	if status.BackendState == "Running" && status.Device.NodeKey != "" {
		return cloudclient.Call(ctx, cfg.Cloud.BaseURL, "/agent/reconcile", cfg.Cloud.DeviceToken, map[string]any{"node_key": status.Device.NodeKey}, &heartbeat)
	}
	return nil
}
func (s *Server) CloudLoop(ctx context.Context) {
	timer := time.NewTicker(3 * time.Second)
	defer timer.Stop()
	next := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.cloudMu.RLock()
			bound := s.Config.Cloud.Bound
			s.cloudMu.RUnlock()
			if bound && time.Now().Before(next) {
				continue
			}
			err := s.syncCloud(ctx)
			s.cloudMu.Lock()
			if err != nil {
				s.cloudError = err.Error()
			} else {
				s.cloudError = ""
			}
			s.cloudMu.Unlock()
			next = time.Now().Add(30 * time.Second)
		}
	}
}
