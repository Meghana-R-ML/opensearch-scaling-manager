import requests
import json
import sys
import time
from requests.auth import HTTPBasicAuth
url = "http://"+sys.argv[4]+":9200/_nodes/"+sys.argv[3]+"/stats/indices"
docs_count = 1
while docs_count != 0:
  response = requests.get(url,auth = HTTPBasicAuth(sys.argv[1],sys.argv[2]))
  json_obj = response.json()
  node_id= list(json_obj['nodes'].keys())
  docs_count= json_obj['nodes'][node_id[0]]['indices']['docs']['count']
  print(docs_count)
  time.sleep(5)
