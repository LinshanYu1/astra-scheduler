# 04-slo-risk-avoidance

## Pods
NAME                                        READY   STATUS    RESTARTS   AGE   IP            NODE      NOMINATED NODE   READINESS GATES
eval-risk-sensitive-chat-7d45bb7568-xdgbn   1/1     Running   0          35s   10.244.0.18   master0   <none>           <none>

## Allocations
NAME                                                 WORKLOAD                   NAMESPACE    NODE      TYPE        PHASE     AGE
eval-risk-sensitive-chat-7d45bb7568-xdgbn-906dfd31   eval-risk-sensitive-chat   astra-eval   master0   preferred   Applied   35s

## Node Profiles
NAME      NODE      BACKEND   PHASE   GPU   KVCACHE   AGE
master0   master0   fake      Ready   1     32        4h33m
master1   master1   fake      Ready   2     48        52m
worker0   worker0   fake      Ready   4     128       52m
worker1   worker1   fake      Ready   3     96        52m
