# 02-time-window-priority

## Pods
NAME                                   READY   STATUS    RESTARTS   AGE   IP            NODE      NOMINATED NODE   READINESS GATES
eval-live-moderation-5ff9cd648-z9fjj   1/1     Running   0          35s   10.244.0.17   master0   <none>           <none>
eval-office-agent-654bd9764-4l5hf      1/1     Running   0          35s   10.244.4.60   worker1   <none>           <none>

## Allocations
NAME                                            WORKLOAD               NAMESPACE    NODE      TYPE        PHASE     AGE
eval-live-moderation-5ff9cd648-z9fjj-906dfd31   eval-live-moderation   astra-eval   master0   preferred   Applied   35s
eval-office-agent-654bd9764-4l5hf-34aa1081      eval-office-agent      astra-eval   worker1   preferred   Applied   35s

## Node Profiles
NAME      NODE      BACKEND   PHASE   GPU   KVCACHE   AGE
master0   master0   fake      Ready   1     16        4h32m
master1   master1   fake      Ready   2     48        50m
worker0   worker0   fake      Ready   4     128       50m
worker1   worker1   fake      Ready   2     64        50m
