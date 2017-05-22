FROM scratch

COPY ./php-fpm-exporter.linux.amd64 /php-fpm-exporter
ENTRYPOINT [ "/php-fpm-exporter" ]
