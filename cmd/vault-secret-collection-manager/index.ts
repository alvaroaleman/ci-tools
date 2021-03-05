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
    const deleteHandler = deleteColectionEventHandler(secretCollection.name)
    deleteCell.addEventListener("click", (e: Event) => deleteHandler());
  }

  const oldTableBody = document.getElementById("secretCollectionTableBody") as HTMLTableSectionElement;
  oldTableBody.parentNode?.replaceChild(newTableBody, oldTableBody);
}
renderCollectionTable(secretCollections);

function createSecretCollection() {
  let input = document.getElementById("name") as HTMLInputElement;
  let name = input.value;
  input.value = "";
  fetch(window.location.protocol + "//" + window.location.host + "/secretcollection/" + name, {method: "PUT"})
  .then(async function (response) {
    if (!response.ok) {
      const responseText = await response.text();
      throw responseText;
    }
    fetchAndRenderSecretCollections();
    hideModal();
  })
  .catch(function (error) {
    displayCreateSecretCollectionError("create secret collection", error);
  });
};

function fetchAndRenderSecretCollections() {
  fetch(window.location.protocol + "//" + window.location.host + "/secretcollection")
  .then(async function(response) {
    const msg = await response.text();
    if (!response.ok) {
      throw msg;
    }
    renderCollectionTable(JSON.parse(msg) as secretCollection[]);
  });
}

function displayCreateSecretCollectionError(attemptedAction: string, msg: string) {
  let div = document.getElementById("modalError") as HTMLDivElement;
  div.innerHTML = `Failed to ${attemptedAction}: ${msg}`;
  div.classList.remove("hidden");
}

function clearCreateSecretCollectionError() {
  let div = document.getElementById("modalError") as HTMLDivElement;
  div.innerHTML = ""
  div.classList.add("hidden");
}

document.getElementById("newCollectionButton")?.addEventListener("click", (e: Event) => {
  document.getElementById("createCollectionInput").classList.remove("hidden");
  showModal();
})

document.getElementById("abortCreateCollectionButton")?.addEventListener("click", (e: Event) => hideModal())

document.addEventListener("keydown", event => {
  const escKeyCode = 27;
  const enterKeyCode = 13;
  if (event.keyCode == escKeyCode) {
    hideModal();
  } else if (event.keyCode == enterKeyCode && !document.getElementById("createCollectionInput")?.classList.contains("hidden")) {
    createSecretCollection();
  };
})

document.getElementById("createCollectionButton").addEventListener("click", (e: Event) => createSecretCollection());

function deleteColectionEventHandler(collectionName: string) {
  return function(){
    let deleteConfirmation = document.getElementById("deleteConfirmation") as HTMLDivElement;
    deleteConfirmation.innerHTML = `Are you sure you want to irreversibly delete the secret collection ${collectionName} and all its content?<br><br>`;

    let cancelButton = document.createElement("button") as HTMLButtonElement;
    cancelButton.type = "button";
    cancelButton.innerHTML = "cancel";
    cancelButton.classList.add("grey-button");
    cancelButton.addEventListener("click", (e: Event) =>{
      hideModal();
    })
    deleteConfirmation.appendChild(cancelButton);

    let confirmButton = document.createElement("button") as HTMLButtonElement;
    confirmButton.type = "button";
    confirmButton.innerHTML = `<i class="fa fa-trash"></i> Delete`;
    confirmButton.classList.add("red-button");
    confirmButton.addEventListener("click", (e: Event) => {
      fetch(window.location.protocol + "//" + window.location.host + "/secretcollection/" + collectionName, {method: "DELETE"})
      .then(async function (response){
        if (!response.ok){
          const msg = await response.text();
          throw msg;
        }
        fetchAndRenderSecretCollections();
        hideModal();
      })
      .catch(function (error) {
        displayCreateSecretCollectionError("delete secret collection", error);
      })
    })
    deleteConfirmation.append("          ");
    deleteConfirmation.appendChild(confirmButton);

    clearCreateSecretCollectionError();
    document.getElementById("deleteConfirmation")?.classList.remove("hidden");
    showModal();
  }
}

function hideModal() {
    document.getElementById("modalContainer")?.classList.add("hidden");
    clearCreateSecretCollectionError();
    let modalContent = document.getElementById("modalContent") as HTMLDivElement;
    for (let child of Array.from(modalContent.children)) {
        child.classList.add("hidden");
    }
}

function showModal() {
    document.getElementById("modalContainer")?.classList.remove("hidden");
}
