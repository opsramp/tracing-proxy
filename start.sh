#!/bin/bash

CLUSTERINFO_PATH='/config/data/infra_clusterinfo.json'
ELASTICACHE_PATH='/config/data/infra_elasticache.json'

#  Sample Format for ${ELASTICACHE_PATH}
#  {
#    "elasticache": {
#      "host": "some_url",
#      "host_ro": "some_url",
#      "port": 6379,
#      "username": "test_user",
#      "password": "xxxxxx",
#      "tls_mode": true,
#      "cluster_mode": "false"
#    }
#  }
#  Sample Format for ${ELASTICACHE_PATH} in case of multi region
#  {
#    "elasticache": [{
#      "us-west-2": {
#        "host": "some_url",
#        "host_ro": "some_url",
#        "port": 6379,
#        "username": "",
#        "password": "xxxxxx",
#        "tls_mode": true,
#        "cluster_mode": "false"
#      },
#      "us-east-2": {
#        "host": "some_url",
#        "host_ro": "some_url",
#        "port": 6379,
#        "username": "",
#        "password": "xxxxxx",
#        "tls_mode": true,
#        "cluster_mode": "false"
#      }
#    }]
#  }

OPSRAMP_CREDS_PATH='/config/data/config_tracing-proxy.json'

#  Sample Format for ${OPSRAMP_CREDS_PATH}
#  {
#    "tracing-proxy": {
#      "traces_api": "some_url",
#      "metrics_api": "some_url",
#      "auth_api": "some_url",
#      "key": "some_key",
#      "secret": "some_secret",
#      "tenant_id": "some_tenant"
#    }
#  }


TRACE_PROXY_CONFIG='/etc/tracing-proxy/final_config.yaml'
TRACE_PROXY_RULES='/etc/tracing-proxy/final_rules.yaml'

# make copy of the config.yaml & rules.yaml to make sure it works if config maps are mounted
cp /etc/tracing-proxy/config.yaml ${TRACE_PROXY_CONFIG}
cp /etc/tracing-proxy/rules.yaml ${TRACE_PROXY_RULES}

if [ -r ${CLUSTERINFO_PATH} ]; then

  CURRENT_REGION=$(jq <${CLUSTERINFO_PATH} -r .clusterinfo.CURRENT_REGION)
  READ_WRITE_REGION=$(jq <${CLUSTERINFO_PATH} -r .clusterinfo.READ_WRITE_REGION)

  while [ "${CURRENT_REGION}" != "${READ_WRITE_REGION}" ]; do sleep 30; done
fi

if [ -r ${ELASTICACHE_PATH} ]; then
  # check if the configuration is a object or array
  TYPE=$(jq <${ELASTICACHE_PATH} -r .elasticache | jq 'if type=="array" then true else false end')
  if [ "${TYPE}" = true ]; then

    if [ -r ${CLUSTERINFO_PATH} ]; then

      CURRENT_REGION=$(jq <${CLUSTERINFO_PATH} -r .clusterinfo.CURRENT_REGION)

      CREDS=$(jq <${ELASTICACHE_PATH} -r .elasticache[0].\""${CURRENT_REGION}"\")

      REDIS_HOST=$(echo "${CREDS}" | jq -r '(.host)+":"+(.port|tostring)')
      REDIS_USERNAME=$(echo "${CREDS}" | jq -r .username)
      REDIS_PASSWORD=$(echo "${CREDS}" | jq -r .password)
      REDIS_TLS_MODE=$(echo "${CREDS}" | jq -r .tls_mode | tr '[:upper:]' '[:lower:]')

      sed -i "s/<REDIS_HOST>/${REDIS_HOST}/g" ${TRACE_PROXY_CONFIG}
      sed -i "s/<REDIS_USERNAME>/${REDIS_USERNAME}/g" ${TRACE_PROXY_CONFIG}
      sed -i "s/<REDIS_PASSWORD>/${REDIS_PASSWORD}/g" ${TRACE_PROXY_CONFIG}
      sed -i "s/<REDIS_TLS_MODE>/${REDIS_TLS_MODE}/g" ${TRACE_PROXY_CONFIG}

    fi


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

  TRACES_API=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".traces_api')
  METRICS_API=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".metrics_api')
  AUTH_API=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".auth_api')
  KEY=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".key')
  SECRET=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".secret')
  TENANT_ID=$(jq <${OPSRAMP_CREDS_PATH} -r '."tracing-proxy".tenant_id')

  sed -i "s*<OPSRAMP_TRACES_API>*${TRACES_API}*g" ${TRACE_PROXY_CONFIG}
  sed -i "s*<OPSRAMP_METRICS_API>*${METRICS_API}*g" ${TRACE_PROXY_CONFIG}
  sed -i "s*<OPSRAMP_API>*${AUTH_API}*g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<KEY>/${KEY}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<SECRET>/${SECRET}/g" ${TRACE_PROXY_CONFIG}
  sed -i "s/<TENANT_ID>/${TENANT_ID}/g" ${TRACE_PROXY_CONFIG}
fi

# start the application
exec /usr/bin/tracing-proxy -c /etc/tracing-proxy/final_config.yaml -r /etc/tracing-proxy/final_rules.yaml
