# Test Output

> Note: this file was added manually, not by the test run
> This is to help review the PR

```text
make nfr-test
NFR=true bash scripts/run-tests-gcp-vm.sh
vars.env                                                                                                                                         100%  945    13.4KB/s   00:00
go run github.com/onsi/ginkgo/v2/ginkgo --randomize-all --randomize-suites --keep-going --fail-on-pending --trace -r -v \
--label-filter "nfr" --label-filter "scale" ./suite --  --gateway-api-version=1.0.0  \
--gateway-api-prev-version=1.0.0  --image-tag=edge --version-under-test= \
--plus-enabled=false --ngf-image-repo=gcr.io/<REDACTED>/michael/scale/nginx-gateway-fabric --nginx-image-repo=gcr.io/<REDACTED>/michael/scale/nginx-gateway-fabric/nginx --nginx-plus-image-repo=gcr.io/<REDACTED>/michael/scale/nginx-gateway-fabric/nginx \
--pull-policy=Always --k8s-version=latest  --service-type=LoadBalancer \
--is-gke-internal-lb=true
Running Suite: NGF System Tests - /home/username/nginx-gateway-fabric/tests/suite
=================================================================================
Random Seed: 1714674847 - will randomize all specs

Will run 5 of 15 specs
------------------------------
[BeforeSuite]
/home/username/nginx-gateway-fabric/tests/suite/system_suite_test.go:246
[BeforeSuite] PASSED [2.492 seconds]
------------------------------
S
------------------------------
Scale test scales HTTP listeners to 64 [nfr, scale]
/home/username/nginx-gateway-fabric/tests/suite/scale_test.go:575
• [298.329 seconds]
------------------------------
Scale test scales HTTPS listeners to 64 [nfr, scale]
/home/username/nginx-gateway-fabric/tests/suite/scale_test.go:603
• [263.231 seconds]
------------------------------
Scale test scales HTTP routes to 1000 [nfr, scale]
/home/username/nginx-gateway-fabric/tests/suite/scale_test.go:631
• [670.689 seconds]
------------------------------
Scale test scales upstream servers to 648 [nfr, scale]
/home/username/nginx-gateway-fabric/tests/suite/scale_test.go:659
• [166.598 seconds]
------------------------------
Scale test scale HTTP matches [nfr, scale]
/home/username/nginx-gateway-fabric/tests/suite/scale_test.go:674
• [261.064 seconds]
------------------------------
SSSSSSSSS
------------------------------
[AfterSuite]
/home/username/nginx-gateway-fabric/tests/suite/system_suite_test.go:275
[AfterSuite] PASSED [60.536 seconds]
------------------------------

Ran 5 of 15 Specs in 1722.941 seconds
SUCCESS! -- 5 Passed | 0 Failed | 0 Pending | 10 Skipped
PASS
```
