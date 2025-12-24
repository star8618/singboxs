package clashapi

import (
"net/http"

"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"

"github.com/go-chi/render"
)

// TrafficClassification 流量分类统计
type TrafficClassification struct {
	DirectUpload   int64 `json:"direct_upload"`
	DirectDownload int64 `json:"direct_download"`
	ProxyUpload    int64 `json:"proxy_upload"`
	ProxyDownload  int64 `json:"proxy_download"`
	TotalUpload    int64 `json:"total_upload"`
	TotalDownload  int64 `json:"total_download"`
}

func trafficClassification(trafficManager *trafficontrol.Manager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		directUp, directDown := trafficManager.DirectTotal()
		proxyUp, proxyDown := trafficManager.ProxyTotal()
		totalUp, totalDown := trafficManager.Total()

		render.JSON(w, r, TrafficClassification{
DirectUpload:   directUp,
DirectDownload: directDown,
ProxyUpload:    proxyUp,
ProxyDownload:  proxyDown,
TotalUpload:    totalUp,
TotalDownload:  totalDown,
})
	}
}
