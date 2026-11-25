package middleware

import (
 "fmt"
 "net/http/httptest"
 "testing"

 "github.com/gin-gonic/gin"
)

func TestQuantumF3CapacityAdmitsNewClient(t *testing.T) {
 gin.SetMode(gin.TestMode)
 r:=gin.New()
 if err:=r.SetTrustedProxies(nil);err!=nil {t.Fatal(err)}
 r.GET("/",QuantumRateLimit(),func(c *gin.Context){c.Status(204)})
 for n:=0;n<4097;n++ {
  req:=httptest.NewRequest("GET","/",nil)
  req.RemoteAddr=fmt.Sprintf("10.%d.%d.%d:1000", n/65536,(n/256)%256,n%256)
  out:=httptest.NewRecorder();r.ServeHTTP(out,req)
  if out.Code!=204 {t.Fatalf("new client %d rejected: %d",n,out.Code)}
 }
}

func TestQuantumF3IPv6PrefixSharesBudget(t *testing.T) {
 gin.SetMode(gin.TestMode)
 r:=gin.New()
 if err:=r.SetTrustedProxies(nil);err!=nil {t.Fatal(err)}
 r.GET("/",QuantumRateLimit(),func(c *gin.Context){c.Status(204)})
 for n:=0;n<121;n++ {
  req:=httptest.NewRequest("GET","/",nil)
  req.RemoteAddr=fmt.Sprintf("[2001:db8:1:2::%x]:1000",n+1)
  out:=httptest.NewRecorder();r.ServeHTTP(out,req)
  want:=204;if n==120 {want=429}
  if out.Code!=want {t.Fatalf("prefix request %d got %d want %d",n,out.Code,want)}
 }
}
