# Classification of Spans

Trace Proxy classifies spans by adding two attributes

| attribute                | description                                                                                                                                                                                                                                          | possible values                   |
|--------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------|
| transaction.type         | usually classifies the span as web (all the REST API, RPC calls from various frameworks, Load Balancers, Web Servers etc) or non-web transactions (like internal schedule jobs, cleanup jobs etc)                                                    | - web<br/> - non-web              |
| transaction.category     | classifying the spans based on the source they originate from, if the span comes from a database then its classified as Database, if it comes from a rpc system then we classify as RPC System and so on                                             | - HTTP<br/> - Database<br/> - ... |
| transaction.sub_category | further classifying the spans based on attributes, if the span comes from a database then its classified based on which database it originated from based on db.system, if it comes from a rpc system then we classify based on rpc.system and so on | - mysql<br/> - kafka<br/> - ...   |                                                                                                                                                                                                                                                                |                                 |

# transaction.type

This label classifies the span as web or non-web transaction
> **Note:**
> By default all the spans are classified as "non-web"

- **web:** spans which originate from an HTTP or RPC call are usually classified as web (internally we check for spans
  with attributes starting with "http.", ".rpc" or "user_agent.")

# transaction.category

- HTTP
- Database
- Messaging queues
- RPC Systems
- Object Store
- Exceptions
- FAAS (Function as a service)
- Feature Flag
- Programming Language

# transaction.sub_category

The Priority order of classification is as follows

- "http.request.method"
- "db.system"
- "messaging.system"
- "rpc.system"
- "aws.s3.bucket"
- "exception.type"
- "faas.trigger"
- "feature_flag.key"
- "telemetry.sdk.language"

## HTTP

we classify http requests based on the attribute "http.request.method"

example values are: GET; POST; HEAD

## Database

we classify DB requests based on the attribute "db.system"

example values can be found at: https://opentelemetry.io/docs/specs/semconv/database/database-spans/

## Messaging Queues

we classify messaging queues based on the attribute "messaging.system"

example values are: kafka, nats etc

## RPC Systems

we classify RPC Systems based on the attribute "rpc.system"

example values are: grpc, apache arrow etc

## Object Store

right now we only classify aws s3 buckets based on the key "aws.s3.bucket"

## Exceptions

we classify exceptions based on the attribute "exception.type"

## FAAS (Function as a service)

classified based on attribute "faas.trigger"

## Feature Flag

classified based on attribute "feature_flag.key"

## Programming Language

classified based on attribute "telemetry.sdk.language"