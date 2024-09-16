.PHONY: terraform install certs copy-scripts install-bins update-configs

terraform:
	terraform apply -var-file pglass-s2c.tfvars

install: install-bins update-configs

copy-scripts:
	./scp.sh ./scripts /home/ec2-user/
	echo 'chmod +x ./scripts/*' | ./ssh.sh

install-bins: copy-scripts
	echo './scripts/install-bins.sh' | ./ssh.sh
	./scp-s2s-proxy-bin.sh

update-configs: copy-scripts
	./scp.sh ./certs /home/ec2-user/
	echo 'sudo rm -rf /etc/temporal/certs; sudo mv /home/ec2-user/certs/ /etc/temporal/' | ./ssh.sh
	echo './scripts/write-configs.sh' | ./ssh.sh

certs:
	rm -rf ./certs
	mkdir -p ./certs
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-host-tls --template '{{index .data "tls.crt"}}' | base64 -d > ./certs/tls.crt
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-host-tls --template '{{index .data "tls.key"}}' | base64 -d > ./certs/tls.key
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-trust-domain-ca --template '{{index .data "tls.crt"}}' | base64 -d > ./certs/ca.crt
