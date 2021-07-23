ARG OPERATORSDK_VERSION

FROM quay.io/operator-framework/helm-operator:${OPERATORSDK_VERSION}

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts  ${HOME}/helm-charts
WORKDIR ${HOME}
