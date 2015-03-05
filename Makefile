all:
	@ echo "########################## BUILD ############################################"
	@ echo && echo && echo && echo
	@ echo && echo && echo && echo
	@ echo && echo && echo && echo
	@ echo && echo && echo && echo
	@cd cmd/isolate && go build
	cmd/isolate/isolate batcharchive --dump-json dump.json test.isolated.gen.json
test:
	@ echo && echo && echo && echo
	@ echo "##########################  TEST ############################################"
	@cd cmd/isolate && go test

