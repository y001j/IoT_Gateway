
package main
import (
    "fmt"
    "github.com/spf13/viper"
)
func main() {
    v := viper.New()
    v.SetConfigFile("config_rule_engine_test.yaml")
    v.SetConfigType("yaml")
    if err := v.ReadInConfig(); err \!= nil {
        fmt.Printf("Error reading config: %v\n", err)
        return
    }
    fmt.Printf("web_ui.enabled: %v\n", v.GetBool("web_ui.enabled"))
    fmt.Printf("web_ui.port: %v\n", v.GetInt("web_ui.port"))
}
