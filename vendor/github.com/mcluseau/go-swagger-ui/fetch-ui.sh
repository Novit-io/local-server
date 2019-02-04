#! /bin/sh

v=3.20.6

file=v$v.tar.gz

[ -e swagger-ui/$file ] ||
    wget -P swagger-ui https://github.com/swagger-api/swagger-ui/archive/$file

rm -fr swagger-ui/dist
tar zxvf swagger-ui/$file --strip-components=1 -C swagger-ui swagger-ui-$v/dist

sed -i -e '/url:/s,https.*/,/,' swagger-ui/dist/index.html