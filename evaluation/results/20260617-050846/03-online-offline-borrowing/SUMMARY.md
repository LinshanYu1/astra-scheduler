# 03-online-offline-borrowing

## Pods
NAME                                    READY   STATUS    RESTARTS   AGE   IP            NODE      NOMINATED NODE   READINESS GATES
eval-batch-embedding-5fc67bd8cd-bcsgs   1/1     Running   0          35s   10.244.4.61   worker1   <none>           <none>
eval-chat-service-65cf4f5c6-9f94j       1/1     Running   0          35s   10.244.2.66   worker0   <none>           <none>

## Allocations
NAME                                             WORKLOAD               NAMESPACE    NODE      TYPE        PHASE     AGE
eval-batch-embedding-5fc67bd8cd-bcsgs-34aa1081   eval-batch-embedding   astra-eval   worker1   preferred   Applied   35s
eval-chat-service-65cf4f5c6-9f94j-b13292c5       eval-chat-service      astra-eval   worker0   preferred   Applied   35s

## Node Profiles
NAME      NODE      BACKEND   PHASE   GPU   KVCACHE   AGE
master0   master0   fake      Ready   2     64        4h32m
master1   master1   fake      Ready   2     48        51m
worker0   worker0   fake      Ready   3     88        51m
worker1   worker1   fake      Ready   2     80        51m
