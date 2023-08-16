#!/bin/bash

ELASTICACHE_PATH='/config/data/infra_elasticache.json'

#  Sample Format for ${ELASTICACHE_PATH}
#  {
#    "elasticache": {
#      "host": "master.testing-non-cluster.89rows.usw2.cache.amazonaws.com",
#      "host_ro": "replica.testing-non-cluster.89rows.usw2.cache.amazonaws.com",
#      "port": 6379,
#      "username": "test_user",
#      "password": "xxxxxx",
#      "tls_mode": true,
#      "cluster_mode": "false"
#    }
#  }

OPSRAMP_CREDS_PATH='/config/data/opsramp_creds.json'

#  Sample Format for ${OPSRAMP_CREDS_PATH}
#  {
#    "traces_api": "test.opsramp.net",
#    "metrics_api": "test.opsramp.net",
#    "auth_api": "test.opsramp.net",
#    "key": "sdjfnsakdflasdflksjdkfjsdklfjals",
#    "secret": "***REMOVED***",
#    "tenant_id": "123e-fsdf-4r234r-dfbfsdbg"
#  }


TRACE_PROXY_CONFIG='/etc/tracing-proxy/final_config.yaml'

# make copy of the config.yaml file
cp /etc/tracing-proxy/config.yaml ${TRACE_PROXY_CONFIG}

if [ -r ${ELASTICACHE_PATH} ]; then
  # check if the configuration is a object or array
  TYPE=$(jq <${ELASTICACHE_PATH} -r .elasticache | jq 'if type=="array" then true else false end')
  if [ "${TYPE}" = true ]; then
    echo "implement me"
  else
    REDIS_HOST=$(jq <${ELASTICACHE_PATH} -r '(.elasticache.host)+":"+(.elasticache.port|tostring)')
    REDIS_USERNAME=$(jq <${ELASTICACHE_PATH} -r .elasticache.username)
    REDIS_PASSWORD=$(jq <${ELASTICACHE_PATH} -r .elasticache.password)
    REDIS_TLS_MODE=$(jq <${ELASTICACHE_PATH} -r .elasticache.tls_mode | tr '[:upper:]' '[:lower:]')

    sed -i "s/<REDIS_HOST>/${REDIS_HOST}/g" ${TRACE_PROXY_CONFIG}
    sed -i "s/<REDIS_USERNAME>/${REDIS_USERNAME}/g" ${TRACE_PROXY_CONFIG}
    sed -i "s/<REDIS_PASSWORD>/${REDIS_PASSWORD}/g" ${TRACE_PROXY_CONFIG}
    sed -i "s/<REDIS_TLS_MODE>/${REDIS_TLS_MODE}/g" ${TRACE_PROXY_CONFIG}
  fi
fi

if [ -r ${OPSRAMP_CREDS_PATH} ]; then

  TRACES_API=$(jq <${OPSRAMP_CREDS_PATH} -r .traces_api)
  METRICS_API=$(jq <${OPSRAMP_CREDS_PATH} -r .metrics_api)
  AUTH_API=$(jq <${OPSRAMP_CREDS_PATH} -r .auth_api)
  KEY=$(jq <${OPSRAMP_CREDS_PATH} -r .key)
  SECRET=$(jq <${OPSRAMP_CREDS_PATH} -r .secret)
  TENANT_ID=$(jq <${OPSRAMP_CREDS_PATH} -r .tenant_id)

  sed -i "s/<OPSRAMP_TRACES_API>/${TRACES_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<OPSRAMP_METRICS_API>/${METRICS_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<OPSRAMP_API>/${AUTH_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<KEY>/${KEY}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<SECRET>/${SECRET}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<TENANT_ID>/${TENANT_ID}/g" ${TRACE_PROXY_CONFIG}
fi

# start the application
exec /usr/bin/tracing-proxy -c /etc/tracing-proxy/final_config.yaml -r /etc/tracing-proxy/rules.yaml
