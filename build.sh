export AGP_JWT_SECRET='替换为长随机字符串'
export BOOTSTRAP_SUPERADMIN_USERNAME='admin'
export BOOTSTRAP_SUPERADMIN_PASSWORD='替换为强密码'

export PRIMARY_GROUP_CODE='agape-a'
export PRIMARY_GROUP_NAME='AGAPE A组'
export PRIMARY_GROUP_DEFAULT_PASSWORD='Abc12345'
export PRIMARY_CONFIG_PATH='/absolute/path/to/config.json'
export PRIMARY_RECORDS_PATH='/absolute/path/to/records.json'

./scripts/deploy-oneclick.sh
