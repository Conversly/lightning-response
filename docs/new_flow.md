1. route : 

/response

2. controller : 

Validates web_id + origin_url mapping. : use api key map from apikey-map.go file

query field have whole json array of prevous conversation in request payload.



3. service : 

checks graph cache

if not found : 

load chatbot config using chatbot id from postgres.  : postgres.go file have function for it. 

{
    	&info.ID,
		&info.Name,
		&info.Description,
		&info.SystemPrompt,
}
later we will add tools config in it too

4. create graphs.

for now only rag tool is available. implementation of actual retreiver is in rag/retreiver.go. 

start the Eino graph

get the final output, 

response will look like this : 

{
    response : "",
    citation : []string,
    success : true/false
}


then in background we will save the messages : 

{
    unique_client_id,
    chatbotid
    message : 
    role : user/assistant
    citations []string
}

