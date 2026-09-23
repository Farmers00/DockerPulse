package updater

import (
	"context"
	"testing"
)

func TestVariousImages(t *testing.T) {
	u := NewUpdateChecker(nil)
	ctx := context.Background()

	images := []string{
		"pihole/pihole:latest",
		"adguard/adguardhome:latest",
		"vaultwarden/server:latest",
		"louislam/uptime-kuma:latest",
		"jc21/nginx-proxy-manager:latest",
		"cloudflare/cloudflared:latest",
		"homeassistant/home-assistant:stable",
		"jellyfin/jellyfin:latest",
		"syncthing/syncthing:latest",
		"portainer/portainer-ce:latest",
		"lscr.io/linuxserver/wireguard:latest",
		"ghcr.io/farmers00/dockerpulse:latest",
	}

	for _, img := range images {
		res, err := u.CheckImage(ctx, img, "sha256:1111111111111111111111111111111111111111111111111111111111111111", nil)
		if err != nil {
			t.Errorf("[FAIL] %s: %v", img, err)
		} else {
			t.Logf("[PASS] %s: hasUpdate=%v, remoteDigest=%s", img, res.HasUpdate, res.RemoteDigest)
		}
	}
}
