syntax = "proto3";

package clash.hub.route;
option csharp_namespace = "Clash.Hub.Route";
option go_package = "github.com/Dreamacro/clash/hub/route";
option java_package = "com.clash.hub.route";
option java_multiple_files = true;

// Domain for routing decision.
message Domain {
  // Type of domain value.
  enum Type {
    // The value is used as is.
    Plain = 0;
    // The value is used as a regular expression.
    Regex = 1;
    // The value is a root domain.
    Domain = 2;
    // The value is a domain.
    Full = 3;
  }

  // Domain matching type.
  Type type = 1;

  // Domain value.
  string value = 2;

  message Attribute {
    string key = 1;

    oneof typed_value {
      bool bool_value = 2;
      int64 int_value = 3;
    }
  }

  // Attributes of this domain. May be used for filtering.
  repeated Attribute attribute = 3;
}

message GeoSite {
  string country_code = 1;
  repeated Domain domain = 2;
}

message GeoSiteList {
  repeated GeoSite entry = 1;
}
