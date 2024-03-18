# Source: https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/ApiextensionsV1Api.md

from __future__ import print_function
import kubernetes.client
from kubernetes.config import load_kube_config
from kubernetes.client.rest import ApiException
from pprint import pprint

load_kube_config()

# Enter a context with an instance of the API kubernetes.client
with kubernetes.client.ApiClient() as api_client:
    # Create an instance of the API class
    api_instance = kubernetes.client.ApiextensionsV1Api(api_client)
    body = kubernetes.client.V1CustomResourceDefinition() # V1CustomResourceDefinition | 
    pretty = 'pretty_example' # str | If 'true', then the output is pretty printed. (optional)
    dry_run = 'dry_run_example' # str | When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processed (optional)
    field_manager = 'field_manager_example' # str | fieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. (optional)
    field_validation = 'field_validation_example' # str | fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered. (optional)

    try:
        api_response = api_instance.create_custom_resource_definition(body, pretty=pretty, dry_run=dry_run, field_manager=field_manager, field_validation=field_validation)
        pprint(api_response)
    except ApiException as e:
        print("Exception when calling ApiextensionsV1Api->create_custom_resource_definition: %s\n" % e)