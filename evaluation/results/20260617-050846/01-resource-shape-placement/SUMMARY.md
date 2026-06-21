# 01-resource-shape-placement

## Pods
NAME                                      READY   STATUS    RESTARTS   AGE   IP            NODE      NOMINATED NODE   READINESS GATES
eval-decode-chat-7c58f7b588-tcxlk         1/1     Running   0          35s   10.244.2.64   worker0   <none>           <none>
eval-kv-agent-74f59b67b4-j5db8            1/1     Running   0          35s   10.244.2.65   worker0   <none>           <none>
eval-prefill-embedding-6b879fb958-wl88g   1/1     Running   0          35s   10.244.4.59   worker1   <none>           <none>

## Allocations
NAME                                               WORKLOAD                 NAMESPACE    NODE      TYPE        PHASE     AGE
eval-decode-chat-7c58f7b588-tcxlk-b13292c5         eval-decode-chat         astra-eval   worker0   preferred   Applied   35s
eval-kv-agent-74f59b67b4-j5db8-b13292c5            eval-kv-agent            astra-eval   worker0   preferred   Applied   35s
eval-prefill-embedding-6b879fb958-wl88g-34aa1081   eval-prefill-embedding   astra-eval   worker1   preferred   Applied   35s

## Node Profiles
NAME      NODE      BACKEND   PHASE   GPU   KVCACHE   AGE
master0   master0   fake      Ready   2     64        4h31m
master1   master1   fake      Ready   2     48        50m
worker0   worker0   fake      Ready   2     40        50m
worker1   worker1   fake      Ready   2     84        50m
