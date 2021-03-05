interface secretCollection {
  name:    string;
  path:    string;
  members: string[];
}

const secretCollections: secretCollection[] = JSON.parse(document.getElementById('secretcollections').innerHTML);

function renderCollectionTable(data: secretCollection[]) {
  if (data === null) {
    return;
  }
  const newTableBody = document.createElement("tbody") as HTMLTableSectionElement;
  newTableBody.id = "secretCollectionTableBody";
  for (let secretCollection of data){
    const row = newTableBody.insertRow();
    row.insertCell().innerHTML = secretCollection.name;
    row.insertCell().innerHTML = secretCollection.path;
    row.insertCell().innerHTML = secretCollection.members.toString();
    let deleteCell = row.insertCell();
    deleteCell.innerHTML = `<button class="red-button"><i class="fa fa-trash"></i> Delete</button>`;
    deleteCell.addEventListener("click", (e: Event) => {
      let deleteConfirmation = document.getElementById("deleteConfirmation") as HTMLDivElement;
      deleteConfirmation.innerHTML = `Are you sure you want to irreversibly delete the secret collection ${secretCollection.name} and all its content?<br><br>`;

      let cancelButton = document.createElement("button") as HTMLButtonElement;
      cancelButton.type = "button";
      cancelButton.innerHTML = "cancel";
      cancelButton.classList.add("grey-button");
      cancelButton.addEventListener("click", (e: Event) =>{
        document.getElementById("createCollection")?.classList.add("hidden");
        document.getElementById("deleteConfirmation")?.classList.add("hidden");
      })
      deleteConfirmation.appendChild(cancelButton);

      let confirmButton = document.createElement("button") as HTMLButtonElement;
      confirmButton.type = "button";
      confirmButton.innerHTML = `<i class="fa fa-trash"></i> Delete`;
      confirmButton.classList.add("red-button");
      confirmButton.addEventListener("click", (e: Event) => {
        fetch(window.location.protocol + "//" + window.location.host + "/secretcollection/" + secretCollection.name, {method: "DELETE"})
        .then(function (response){
          if (response.ok){
            fetchAndRenderSecretCollections();
            (document.getElementById("createCollection") as HTMLDivElement).classList.add("hidden");
          } else {
            return response.text();
          }
        })
        .then(function (errMsg: string) {
          displayCreateSecretCollectionError("delete secret collection", errMsg);
        })
        .catch(function (error) {
          displayCreateSecretCollectionError("delete secret collection", error);
        })
      })
      deleteConfirmation.append("          ");
      deleteConfirmation.appendChild(confirmButton);

      clearCreateSecretCollectionError();
      document.getElementById("createCollectionInput")?.classList.add("hidden");
      document.getElementById("deleteConfirmation")?.classList.remove("hidden");
      document.getElementById("createCollection")?.classList.remove("hidden");
    });
  }

  const oldTableBody = document.getElementById("secretCollectionTableBody") as HTMLTableSectionElement;
  oldTableBody.parentNode?.replaceChild(newTableBody, oldTableBody);
}
renderCollectionTable(secretCollections);

function createSecretCollection(){
  let input = document.getElementById("name") as HTMLInputElement;
  let name = input.value;
  input.value = "";
  fetch(window.location.protocol + "//" + window.location.host + "/secretcollection/" + name, {method: "PUT"})
  .then(function (response) {
    if (response.ok) {
      fetchAndRenderSecretCollections();
      (document.getElementById("createCollection") as HTMLDivElement).classList.add("hidden");
    } else {
      return response.text();
    };
  })
  .then(function (errMsg: string){
    displayCreateSecretCollectionError("create secret collection", errMsg)  ;
  })
  .catch(function (error) {
    displayCreateSecretCollectionError("create secret collection", error);
  });
};

function fetchAndRenderSecretCollections() {
  fetch(window.location.protocol + "//" + window.location.host + "/secretcollection/")
  .then(function(response) {
    if (response.ok) {
      return response.text();
    }
  })
  .then(function(data: string) {
    renderCollectionTable(JSON.parse(data) as secretCollection[]);
  });
}

function displayCreateSecretCollectionError(attemptedAction: string, msg: string) {
  let div = document.getElementById("createCollectionError") as HTMLDivElement;
  div.innerHTML = `Failed to ${attemptedAction}: ${msg}`;
  div.classList.remove("hidden");
}

function clearCreateSecretCollectionError(){
  let div = document.getElementById("createCollectionError") as HTMLDivElement;
  div.innerHTML = ""
  div.classList.add("hidden");
}

document.getElementById("newCollectionButton")?.addEventListener("click", (e: Event) => {
  clearCreateSecretCollectionError();
  document.getElementById("deleteConfirmation")?.classList.add("hidden");
  document.getElementById("createCollectionInput").classList.remove("hidden");
  document.getElementById("createCollection")?.classList.remove("hidden");
})
document.getElementById("abortCreateCollectionButton")?.addEventListener("click", (e: Event) => {
  document.getElementById("createCollection")?.classList.add("hidden");
})
document.addEventListener("keydown", event => {
  // esc
  if (event.keyCode == 27) {
    (document.getElementById("createCollection") as HTMLDivElement).classList.add("hidden");
  };
})

document.getElementById("createCollectionButton").addEventListener("click", (e: Event) => createSecretCollection());

function test(msg: string) function {
  return function(){
     console.log(msg);
  }
}
