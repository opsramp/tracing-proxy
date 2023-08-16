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

OPSRAMP_CREDS_PATH='/config/data/config_opsramp-tracing-proxy-creds.json'

#  Sample Format for ${OPSRAMP_CREDS_PATH}
#  {
#    "opsramp-tracing-proxy-creds": {
#      "traces_api": "***REMOVED***",
#      "metrics_api": "***REMOVED***",
#      "auth_api": "***REMOVED***",
#      "key": "***REMOVED***",
#      "secret": "***REMOVED***",
#      "tenant_id": "***REMOVED***"
#    }
#  }


TRACE_PROXY_CONFIG='/etc/tracing-proxy/final_config.yaml'
TRACE_PROXY_RULES='/etc/tracing-proxy/final_rules.yaml'

# make copy of the config.yaml & rules.yaml to make sure it works if config maps are mounted
cp /etc/tracing-proxy/config.yaml ${TRACE_PROXY_CONFIG}
cp /etc/tracing-proxy/rules.yaml ${TRACE_PROXY_RULES}

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

  TRACES_API=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.traces_api)
  METRICS_API=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.metrics_api)
  AUTH_API=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.auth_api)
  KEY=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.key)
  SECRET=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.secret)
  TENANT_ID=$(jq <${OPSRAMP_CREDS_PATH} -r .opsramp-tracing-proxy-creds.tenant_id)

  sed -i "s/<OPSRAMP_TRACES_API>/${TRACES_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<OPSRAMP_METRICS_API>/${METRICS_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<OPSRAMP_API>/${AUTH_API}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<KEY>/${KEY}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<SECRET>/${SECRET}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<TENANT_ID>/${TENANT_ID}/g" ${TRACE_PROXY_CONFIG}
fi

# start the application
exec /usr/bin/tracing-proxy -c /etc/tracing-proxy/final_config.yaml -r /etc/tracing-proxy/final_rules.yaml
