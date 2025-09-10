FROM ankane/pgvector:latest

RUN echo "deb http://mirrors.aliyun.com/debian/ bookworm main contrib non-free" > /etc/apt/sources.list && \
    echo "deb http://mirrors.aliyun.com/debian/ bookworm-updates main contrib non-free" >> /etc/apt/sources.list && \
    echo "deb http://mirrors.aliyun.com/debian-security/ bookworm-security main contrib non-free" >> /etc/apt/sources.list

# 安装基础依赖
RUN apt-get clean && \
    apt-get update --fix-missing -o Acquire::Retries=3 -o Acquire::http::Timeout="60" && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        make \
        build-essential \
        postgresql-server-dev-all \
        pkg-config \
        wget \
        autoconf \
        automake \
        libtool \
        zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*


RUN set -e && \
    cd /tmp && \
    wget http://www.xunsearch.com/scws/down/scws-1.2.3.tar.bz2 && \
    tar xjf scws-1.2.3.tar.bz2 && \
    cd scws-1.2.3 && \
    ./configure --prefix=/usr/local && \
    make && make install && \
    cd /tmp && rm -rf scws-*
ARG PG_VERSION=15
ENV PG_VERSION=${PG_VERSION}
RUN set -e && \
    git clone --depth 1 https://github.com/amutu/zhparser.git /tmp/zhparser && \
    cd /tmp/zhparser && \
    SCWS_HOME=/usr/local make PG_CONFIG=/usr/lib/postgresql/${PG_VERSION}/bin/pg_config && \
    make PG_CONFIG=/usr/lib/postgresql/${PG_VERSION}/bin/pg_config install && \
    cd / && rm -rf /tmp/zhparser

RUN echo '/usr/local/lib' > /etc/ld.so.conf.d/scws.conf && ldconfig
ENV LD_LIBRARY_PATH /usr/local/lib:$LD_LIBRARY_PATH
