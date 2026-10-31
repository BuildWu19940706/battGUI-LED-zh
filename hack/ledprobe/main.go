// ledprobe is a read-only diagnostic. It never changes charging or LED state.
package main

import (
 "encoding/json"
 "os"
 "github.com/charlie0129/batt/pkg/smc"
)

func main(){
 result:=map[string]any{"readOnly":true}
 c:=smc.New()
 if err:=c.Open();err!=nil {result["error"]=err.Error()}else{
  defer c.Close()
  result["chargeControlMode"]=c.ChargeControlMode()
  result["ledInterfacePresent"]=c.CheckMagSafeExistence()
  state,err:=c.GetMagSafeLedState()
  if err!=nil{result["ledReadError"]=err.Error()}else{result["ledState"]=state}
 }
 encoder:=json.NewEncoder(os.Stdout);encoder.SetIndent("","  ");_ = encoder.Encode(result)
}
