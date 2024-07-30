#!/bin/bash

# Set the kubeconfig environment variable
OVERLAY_KUBECONFIG=$(echo "/src/go.goms.io/aks/rp" | awk 'END{print $0 "/bin/kubeconfig"}')
export KUBECONFIG=$OVERLAY_KUBECONFIG

# Retrieve the metrics server deployment
kubectl get deployment metrics-server -n kube-system -o yaml > metrics-server.yaml

# Backup the current metrics server deployment yaml
cp metrics-server.yaml metrics-server-backup.yaml

# Add the konnectivity agent sidecar container to the metrics server deployment yaml
cat <<EOF >> metrics-server.yaml
        - name: konnectivity-agent
          image: <konnectivity-agent-image>
          imagePullPolicy: IfNotPresent
          command:
            - /proxy-agent
            - --proxy-server-host=chr20aks-jaea17vy.hcp.r3i80kywp.e2e.azmk8s.io
            - --proxy-server-port=443
            - --health-server-port=8082
            - --keepalive-time=30s
            - --agent-key=/certs/client.key
            - --agent-cert=/certs/client.crt
            - --ca-cert=/certs/ca.crt
            - --agent-identifiers=ipv4=$(POD_IP)
            - --alpn-proto=konnectivity
            - -v=2
          env:
            - name: POD_IP
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: status.podIP
          livenessProbe:
            httpGet:
              path: /ready
              port: 8082
              scheme: HTTP
            initialDelaySeconds: 30
            periodSeconds: 60
            timeoutSeconds: 60
            successThreshold: 1
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 8082
              scheme: HTTP
            periodSeconds: 10
            timeoutSeconds: 1
            successThreshold: 1
            failureThreshold: 3
          volumeMounts:
            - mountPath: /certs
              name: certs
              readOnly: true
EOF

# Apply the updated metrics server deployment
kubectl apply -f metrics-server.yaml

# Verify the pods are running
kubectl get pods -n kube-system
