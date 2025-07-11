# Overview

This is a partial copy of [temporalio/temporal at v1.22.2](https://github.com/temporalio/temporal/tree/v1.22.2)

Temporal v1.22 used gogo-protobuf for serialization, which allowed it to store
invalid UTF8 provided from the SDK and workers locally. Temporal 1.23 switched
to google-protobuf, which requires all string data be valid UTF8. The proxy uses
these proto definitions to seamlessly repair broken UTF8 strings in flight
without dropping any of the valid data.
